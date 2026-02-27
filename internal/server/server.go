package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"

	"anytls/internal/alive"
	"anytls/internal/api"
	"anytls/internal/config"
	"anytls/internal/fallback"
	"anytls/internal/ratelimit"
	"anytls/internal/traffic"
	"anytls/internal/user"

	"github.com/sirupsen/logrus"
)

const trafficPersistPath = "/tmp/anytls-traffic.json"

// Server 主服务
type Server struct {
	config         *config.Config
	apiClient      *api.Client
	userManager    *user.Manager
	trafficCounter *traffic.Counter
	speedLimiter   *ratelimit.SpeedLimiter
	connLimiter    *ratelimit.ConnRateLimiter
	aliveTracker   *alive.Tracker
	fallback       *fallback.Handler
	tlsConfig      *tls.Config
	listener       net.Listener
	logger         *logrus.Logger

	// nodeConfig stores the config fetched from API (server_port, intervals, etc.)
	nodeConfig *api.NodeConfig

	wg sync.WaitGroup // tracks active connections
}

// NewServer 创建服务实例
func NewServer(cfg *config.Config) (*Server, error) {
	logger, err := config.SetupLogger(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("初始化日志失败: %w", err)
	}

	tlsCfg, err := LoadTLSConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("加载 TLS 配置失败: %w", err)
	}

	return &Server{
		config:         cfg,
		apiClient:      api.NewClient(cfg.APIHost, cfg.APIToken, cfg.NodeID, cfg.NodeType, logger),
		userManager:    user.NewManager(),
		trafficCounter: traffic.NewCounter(),
		speedLimiter:   ratelimit.NewSpeedLimiter(),
		connLimiter:    ratelimit.NewConnRateLimiter(),
		aliveTracker:   alive.NewTracker(cfg.NodeID),
		fallback:       fallback.NewHandler(cfg.Fallback),
		tlsConfig:      tlsCfg,
		logger:         logger,
	}, nil
}

// Start 启动服务
// 流程：FetchConfig → FetchUsers → 启动 listener → 启动 syncLoop → accept loop
func (s *Server) Start(ctx context.Context) error {
	// 1. 获取节点配置
	nodeConfig, err := s.apiClient.FetchConfig()
	if err != nil {
		return fmt.Errorf("获取节点配置失败: %w", err)
	}
	s.nodeConfig = nodeConfig
	s.logger.WithFields(logrus.Fields{
		"server_port":   nodeConfig.ServerPort,
		"push_interval": nodeConfig.BaseConfig.PushInterval,
		"pull_interval": nodeConfig.BaseConfig.PullInterval,
	}).Info("节点配置已加载")

	// 2. 拉取用户列表
	users, err := s.apiClient.FetchUsers()
	if err != nil {
		return fmt.Errorf("拉取用户列表失败: %w", err)
	}
	if users != nil {
		s.userManager.UpdateUsers(users)
		s.logger.WithField("count", len(users)).Info("用户列表已加载")
	}

	// 3. 确定监听地址：API 返回的 server_port 优先
	listenAddr := s.config.Listen
	if nodeConfig.ServerPort > 0 {
		_, _, err := net.SplitHostPort(listenAddr)
		if err != nil {
			// listenAddr 不含端口，直接拼接
			listenAddr = fmt.Sprintf("%s:%d", listenAddr, nodeConfig.ServerPort)
		} else {
			host, _, _ := net.SplitHostPort(listenAddr)
			listenAddr = fmt.Sprintf("%s:%d", host, nodeConfig.ServerPort)
		}
	}

	// 4. 启动 TCP listener
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("监听 %s 失败: %w", listenAddr, err)
	}
	s.listener = ln
	s.logger.WithField("addr", listenAddr).Info("服务已启动")

	// 5. 启动 syncLoop
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.syncLoop(ctx)
	}()

	// 6. Accept loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			// listener 被关闭时退出循环
			select {
			case <-ctx.Done():
				return nil
			default:
				if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
					return nil
				}
				s.logger.WithError(err).Error("接受连接失败")
				continue
			}
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(ctx, conn)
		}()
	}
}

// LoadTrafficData 从持久化文件加载流量数据
func (s *Server) LoadTrafficData(path string) error {
	return s.trafficCounter.LoadFromFile(path)
}

// Shutdown 优雅关闭
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("正在关闭服务...")

	// 1. 关闭 listener，停止接受新连接
	if s.listener != nil {
		s.listener.Close()
	}

	// 2. 快照流量并上报
	snapshot := s.trafficCounter.Snapshot()
	if len(snapshot) > 0 {
		if err := s.apiClient.PushTraffic(snapshot); err != nil {
			s.logger.WithError(err).Error("关闭时上报流量失败")
			// 上报失败，合并回计数器以便持久化
			s.trafficCounter.Merge(snapshot)
		} else {
			s.logger.WithField("users", len(snapshot)).Info("关闭时流量已上报")
		}
	}

	// 3. 持久化未上报的流量数据
	if err := s.trafficCounter.SaveToFile(trafficPersistPath); err != nil {
		s.logger.WithError(err).Error("持久化流量数据失败")
	}

	// 4. 等待现有连接完成（受 ctx 超时控制）
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("所有连接已关闭")
	case <-ctx.Done():
		s.logger.Warn("等待连接关闭超时")
	}

	s.logger.Info("服务已关闭")
	return nil
}
