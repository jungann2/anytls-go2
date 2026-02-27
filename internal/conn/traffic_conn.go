package conn

import (
	"context"
	"net"

	"anytls/internal/traffic"

	"golang.org/x/time/rate"
)

// TrafficConn 带流量统计和限速的连接包装器
// 包装传递给 session.NewServerSession() 的 TLS 连接，统计包括协议开销和 padding 在内的全部流量
type TrafficConn struct {
	net.Conn
	userID  int
	counter *traffic.Counter
	limiter *rate.Limiter // nil 表示不限速
}

// NewTrafficConn 创建带流量统计和限速的连接包装器
func NewTrafficConn(conn net.Conn, userID int, counter *traffic.Counter, limiter *rate.Limiter) *TrafficConn {
	return &TrafficConn{
		Conn:    conn,
		userID:  userID,
		counter: counter,
		limiter: limiter,
	}
}

// Read 读取数据并统计下载流量
func (c *TrafficConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	if n > 0 {
		c.counter.Add(c.userID, 0, int64(n))
		if c.limiter != nil {
			_ = c.limiter.WaitN(context.Background(), n)
		}
	}
	return
}

// Write 写入数据并统计上传流量
func (c *TrafficConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	if n > 0 {
		c.counter.Add(c.userID, int64(n), 0)
		if c.limiter != nil {
			_ = c.limiter.WaitN(context.Background(), n)
		}
	}
	return
}
