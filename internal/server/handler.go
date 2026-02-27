package server

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"net"
	"runtime/debug"
	"strings"

	"anytls/internal/conn"
	"anytls/proxy"
	"anytls/proxy/padding"
	"anytls/proxy/session"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/uot"
	"github.com/sirupsen/logrus"
)

// handleConnection 处理单个 TLS 连接
// 流程：IP 封禁检查 → TLS 握手 → 读取密码哈希 → 多用户认证 → 设备限制检查 → 创建 TrafficConn → 建立 Session
func (s *Server) handleConnection(ctx context.Context, c net.Conn) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.Errorln("[BUG]", r, string(debug.Stack()))
		}
	}()

	// 1. 提取远程 IP，检查是否被封禁
	remoteIP, _, _ := net.SplitHostPort(c.RemoteAddr().String())
	if s.connLimiter.IsBanned(remoteIP) {
		c.Close()
		return
	}

	// 2. TLS 握手
	tlsConn := tls.Server(c, s.tlsConfig)
	defer tlsConn.Close()

	// 3. 读取首包数据
	b := buf.NewPacket()
	defer b.Release()

	n, err := b.ReadOnceFrom(tlsConn)
	if err != nil {
		s.logger.Debugln("ReadOnceFrom:", err)
		return
	}
	cachedConn := bufio.NewCachedConn(tlsConn, b)

	// 4. 读取 32 字节密码哈希并认证
	passwordHash, err := b.ReadBytes(32)
	if err != nil {
		b.Resize(0, n)
		s.connLimiter.RecordFailure(remoteIP)
		s.fallback.Handle(ctx, cachedConn)
		return
	}

	userEntry := s.userManager.Authenticate(passwordHash)
	if userEntry == nil {
		b.Resize(0, n)
		s.connLimiter.RecordFailure(remoteIP)
		s.logger.WithField("ip", remoteIP).Debug("认证失败")
		s.fallback.Handle(ctx, cachedConn)
		return
	}

	// 5. 读取并跳过 padding
	paddingLenBytes, err := b.ReadBytes(2)
	if err != nil {
		b.Resize(0, n)
		s.connLimiter.RecordFailure(remoteIP)
		s.fallback.Handle(ctx, cachedConn)
		return
	}
	paddingLen := binary.BigEndian.Uint16(paddingLenBytes)
	if paddingLen > 0 {
		_, err = b.ReadBytes(int(paddingLen))
		if err != nil {
			b.Resize(0, n)
			s.connLimiter.RecordFailure(remoteIP)
			s.fallback.Handle(ctx, cachedConn)
			return
		}
	}

	// 6. 检查设备限制
	if userEntry.DeviceLimit > 0 {
		aliveList, err := s.apiClient.FetchAliveList()
		if err != nil {
			s.logger.WithError(err).Warn("获取在线设备数失败，跳过设备限制检查")
		} else if !s.aliveTracker.CheckDeviceLimit(userEntry.ID, userEntry.DeviceLimit, aliveList) {
			s.logger.WithFields(logrus.Fields{
				"user_id":      userEntry.ID,
				"device_limit": userEntry.DeviceLimit,
			}).Info("设备数超限，拒绝连接")
			return
		}
	}

	// 7. 获取限速器并创建 TrafficConn
	limiter := s.speedLimiter.GetLimiter(userEntry.ID, userEntry.SpeedLimit)
	trafficConn := conn.NewTrafficConn(cachedConn, userEntry.ID, s.trafficCounter, limiter)

	// 8. 追踪在线设备
	s.aliveTracker.Track(userEntry.ID, remoteIP)
	defer s.aliveTracker.Remove(userEntry.ID, remoteIP)

	s.logger.WithFields(logrus.Fields{
		"user_id": userEntry.ID,
		"ip":      remoteIP,
	}).Debug("认证成功，建立会话")

	// 9. 创建并运行 Session
	sess := session.NewServerSession(trafficConn, func(stream *session.Stream) {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Errorln("[BUG]", r, string(debug.Stack()))
			}
		}()
		defer stream.Close()

		destination, err := M.SocksaddrSerializer.ReadAddrPort(stream)
		if err != nil {
			s.logger.Debugln("ReadAddrPort:", err)
			return
		}

		if strings.Contains(destination.String(), "udp-over-tcp.arpa") {
			proxyOutboundUoT(ctx, stream, destination)
		} else {
			proxyOutboundTCP(ctx, stream, destination)
		}
	}, &padding.DefaultPaddingFactory)
	sess.Run()
	sess.Close()
}

// proxyOutboundTCP 代理 TCP 出站连接
func proxyOutboundTCP(ctx context.Context, c net.Conn, destination M.Socksaddr) error {
	outbound, err := proxy.SystemDialer.DialContext(ctx, "tcp", destination.String())
	if err != nil {
		logrus.Debugln("proxyOutboundTCP DialContext:", err)
		err = E.Errors(err, N.ReportHandshakeFailure(c, err))
		return err
	}
	err = N.ReportHandshakeSuccess(c)
	if err != nil {
		return err
	}
	return bufio.CopyConn(ctx, c, outbound)
}

// proxyOutboundUoT 代理 UDP-over-TCP 出站连接
func proxyOutboundUoT(ctx context.Context, c net.Conn, destination M.Socksaddr) error {
	request, err := uot.ReadRequest(c)
	if err != nil {
		logrus.Debugln("proxyOutboundUoT ReadRequest:", err)
		return err
	}
	pc, err := net.ListenPacket("udp", "")
	if err != nil {
		logrus.Debugln("proxyOutboundUoT ListenPacket:", err)
		err = E.Errors(err, N.ReportHandshakeFailure(c, err))
		return err
	}
	err = N.ReportHandshakeSuccess(c)
	if err != nil {
		return err
	}
	return bufio.CopyPacketConn(ctx, uot.NewConn(c, *request), bufio.NewPacketConn(pc))
}
