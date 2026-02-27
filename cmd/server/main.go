package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/url"
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
	standalone := flag.Bool("standalone", false, "独立运行模式（不依赖 Xboard）")
	password := flag.String("p", "", "独立模式密码")
	listen := flag.String("l", "", "监听地址（覆盖配置文件）")
	sni := flag.String("sni", "", "TLS SNI（用于生成分享链接）")
	flag.Parse()

	var cfg *config.Config

	if *standalone {
		// 独立模式：不需要配置文件
		if *password == "" {
			fmt.Fprintln(os.Stderr, "独立模式需要指定密码: -p <password>")
			os.Exit(1)
		}
		listenAddr := "0.0.0.0:8443"
		if *listen != "" {
			listenAddr = *listen
		}
		cfg = &config.Config{
			Listen:     listenAddr,
			Standalone: true,
			Password:   *password,
			NodeType:   "anytls",
			Log:        config.LogConfig{Level: "info"},
		}
	} else {
		// Xboard 模式：从配置文件加载
		var err error
		cfg, err = config.LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			os.Exit(1)
		}
		if *listen != "" {
			cfg.Listen = *listen
		}
	}

	// 初始化日志
	logger, err := config.SetupLogger(cfg.Log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
		os.Exit(1)
	}

	logger.WithField("version", util.ProgramVersionName).Info("AnytlsServer 启动中")

	// 创建 Server
	srv, err := server.NewServer(cfg)
	if err != nil {
		logger.WithError(err).Fatal("创建服务失败")
	}

	// 加载持久化流量数据
	if err := srv.LoadTrafficData(trafficPersistPath); err != nil {
		logger.WithError(err).Warn("加载持久化流量数据失败")
	}

	// 启动服务
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	// 独立模式：打印分享链接
	if cfg.Standalone {
		// 等一小会让 listener 启动
		time.Sleep(200 * time.Millisecond)
		printShareLink(cfg, *sni)
	}

	// 处理信号
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

	// 优雅关闭
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.WithError(err).Error("关闭服务失败")
		os.Exit(1)
	}

	logger.Info("服务已安全退出")
}

// printShareLink 打印 anytls:// 分享链接
func printShareLink(cfg *config.Config, sni string) {
	_, port, _ := net.SplitHostPort(cfg.Listen)

	// 尝试获取公网 IP
	host := getPublicIP()
	if host == "" {
		host = "YOUR_SERVER_IP"
	}

	addr := net.JoinHostPort(host, port)
	u := url.URL{
		Scheme: "anytls",
		User:   url.User(cfg.Password),
		Host:   addr,
	}

	q := url.Values{}
	if sni != "" {
		q.Set("sni", sni)
	}
	q.Set("insecure", "1") // 自签名证书需要 insecure
	u.RawQuery = q.Encode()

	fmt.Println()
	fmt.Println("========== AnyTLS 分享链接 ==========")
	fmt.Println(u.String())
	fmt.Println("======================================")
	fmt.Println()
	fmt.Println("FlClash/Clash.Meta 配置:")
	fmt.Printf(`
  - name: "anytls-node"
    type: anytls
    server: %s
    port: %s
    password: "%s"
    udp: true
    skip-cert-verify: true
`, host, port, cfg.Password)
	if sni != "" {
		fmt.Printf("    sni: \"%s\"\n", sni)
	}
	fmt.Println()
}

// getPublicIP 尝试获取公网 IP
func getPublicIP() string {
	conn, err := net.DialTimeout("udp", "8.8.8.8:80", 3*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	ip := localAddr.IP.String()
	// 如果是内网 IP，返回空让用户自己填
	if ip == "127.0.0.1" || ip == "::1" {
		return ""
	}
	return ip
}
