package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"anytls/internal/config"
	"anytls/internal/server"
	"anytls/util"
)

const trafficPersistPath = "/tmp/anytls-traffic.json"

func main() {
	configPath := flag.String("c", "/etc/anytls/config.yaml", "配置文件路径")
	flag.Parse()

	// 1. 加载配置
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}

	// 2. 初始化日志
	logger, err := config.SetupLogger(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("version", util.ProgramVersionName).Info("AnytlsServer 启动中")

	// 3. 创建 Server
	srv, err := server.NewServer(cfg)
	if err != nil {
		logger.WithError(err).Fatal("创建服务失败")
	}

	// 4. 加载持久化流量数据
	if err := srv.LoadTrafficData(trafficPersistPath); err != nil {
		logger.WithError(err).Warn("加载持久化流量数据失败")
	}

	// 5. 启动服务（在 goroutine 中运行，因为 Start 是阻塞的）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// 6. 处理信号（SIGTERM/SIGINT）
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		logger.WithField("signal", sig.String()).Info("收到关闭信号")
	case err := <-errCh:
		if err != nil {
			logger.WithError(err).Fatal("服务异常退出")
		}
		return
	}

	// 7. 优雅关闭：30 秒超时
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("关闭服务失败")
		os.Exit(1)
	}

	logger.Info("服务已安全退出")
}
