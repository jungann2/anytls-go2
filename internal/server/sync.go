package server

import (
	"anytls/internal/api"
	"anytls/proxy/padding"
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// syncLoop 定期同步用户和上报数据
// pull 周期：FetchUsers（ETag）→ UpdateUsers；FetchConfig → 更新 padding；Cleanup 过期封禁
// push 周期：Snapshot 流量 → PushTraffic（失败回滚）；Snapshot alive → PushAlive；PushStatus；SaveToFile
func (s *Server) syncLoop(ctx context.Context) {
	pullInterval := time.Duration(s.nodeConfig.BaseConfig.PullInterval) * time.Second
	pushInterval := time.Duration(s.nodeConfig.BaseConfig.PushInterval) * time.Second

	// 安全默认值
	if pullInterval <= 0 {
		pullInterval = 60 * time.Second
	}
	if pushInterval <= 0 {
		pushInterval = 60 * time.Second
	}

	pullTicker := time.NewTicker(pullInterval)
	pushTicker := time.NewTicker(pushInterval)
	defer pullTicker.Stop()
	defer pushTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-pullTicker.C:
			s.doPull()

		case <-pushTicker.C:
			s.doPush()
		}
	}
}

// doPull 执行一次 pull 周期
func (s *Server) doPull() {
	// 1. 拉取用户列表（支持 ETag，304 时返回 nil,nil）
	users, err := s.apiClient.FetchUsers()
	if err != nil {
		s.logger.WithError(err).Error("拉取用户列表失败")
	} else if users != nil {
		s.userManager.UpdateUsers(users)
		s.logger.WithField("count", len(users)).Info("用户列表已同步")
	}

	// 2. 拉取节点配置并更新 padding
	nodeConfig, err := s.apiClient.FetchConfig()
	if err != nil {
		s.logger.WithError(err).Error("拉取节点配置失败")
	} else {
		s.nodeConfig = nodeConfig
		if len(nodeConfig.PaddingScheme) > 0 {
			rawScheme := api.PaddingSchemeToBytes(nodeConfig.PaddingScheme)
			if !padding.UpdatePaddingScheme(rawScheme) {
				s.logger.Warn("padding scheme 更新失败，格式可能不正确")
			}
		}
	}

	// 3. 清理过期封禁记录
	s.connLimiter.Cleanup()
}

// doPush 执行一次 push 周期
func (s *Server) doPush() {
	// 1. 快照流量并上报
	trafficSnapshot := s.trafficCounter.Snapshot()
	if len(trafficSnapshot) > 0 {
		if err := s.apiClient.PushTraffic(trafficSnapshot); err != nil {
			s.logger.WithError(err).Error("上报流量失败，保留数据待下次上报")
			// 上报失败，合并回计数器
			s.trafficCounter.Merge(trafficSnapshot)
		} else {
			s.logger.WithField("users", len(trafficSnapshot)).Debug("流量数据已上报")
		}
	}

	// 2. 快照在线用户并上报
	aliveSnapshot := s.aliveTracker.Snapshot()
	if len(aliveSnapshot) > 0 {
		if err := s.apiClient.PushAlive(aliveSnapshot); err != nil {
			s.logger.WithError(err).Error("上报在线数据失败")
		} else {
			s.logger.WithFields(logrus.Fields{
				"users": len(aliveSnapshot),
			}).Debug("在线数据已上报")
		}
	}

	// 3. 收集系统信息并上报节点状态
	nodeStatus, err := collectSystemInfo()
	if err != nil {
		s.logger.WithError(err).Error("收集系统信息失败")
	} else {
		if err := s.apiClient.PushStatus(nodeStatus); err != nil {
			s.logger.WithError(err).Error("上报节点状态失败")
		} else {
			s.logger.Debug("节点状态已上报")
		}
	}

	// 4. 持久化流量数据
	if err := s.trafficCounter.SaveToFile(trafficPersistPath); err != nil {
		s.logger.WithError(err).Error("持久化流量数据失败")
	}
}
