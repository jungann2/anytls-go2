package fallback

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/sirupsen/logrus"
)

// Handler fallback 处理器
// 认证失败时将连接转发到正常网站，伪装为普通 HTTPS 流量
type Handler struct {
	target string // 目标地址，如 "127.0.0.1:80"
}

// NewHandler 创建 fallback 处理器
// target 为空时 Handle 会直接关闭连接
func NewHandler(target string) *Handler {
	return &Handler{target: target}
}

// Handle 将连接转发到目标
func (h *Handler) Handle(ctx context.Context, conn net.Conn) {
	if h.target == "" {
		conn.Close()
		return
	}

	dialer := net.Dialer{Timeout: 5 * time.Second}
	remote, err := dialer.DialContext(ctx, "tcp", h.target)
	if err != nil {
		logrus.WithField("target", h.target).Debug("fallback 拨号失败: ", err)
		conn.Close()
		return
	}

	go func() {
		defer remote.Close()
		defer conn.Close()
		io.Copy(remote, conn)
	}()

	defer remote.Close()
	defer conn.Close()
	io.Copy(conn, remote)
}
