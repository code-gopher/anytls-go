package main

import (
	"anytls/proxy/padding"
	"anytls/proxy/session"
	"context"
	"crypto/tls"
	"encoding/binary"
	"net"
	"runtime/debug"
	"strings"

	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	"github.com/sirupsen/logrus"
)

// handleTcpConnection 处理一个入站 TCP 连接的完整生命周期：
// TLS 握手 -> 认证 -> 会话复用 -> 代理请求
func handleTcpConnection(ctx context.Context, c net.Conn, s *myServer) {
	defer func() {
		if r := recover(); r != nil {
			logrus.Errorln("[BUG]", r, string(debug.Stack()))
		}
	}()

	// TLS 握手
	c = tls.Server(c, s.tlsConfig)
	defer c.Close()

	// 读取首个数据包（包含认证信息）
	b := buf.NewPacket()
	defer b.Release()

	n, err := b.ReadOnceFrom(c)
	if err != nil {
		logrus.Debugln("ReadOnceFrom:", err)
		return
	}
	c = bufio.NewCachedConn(c, b)

	// 读取并验证 sha256(password/uuid)（32 字节）
	passwordHashBytes, err := b.ReadBytes(32)
	if err != nil || !isValidAuth(passwordHashBytes, s, n, c, b) {
		return
	}

	// 读取并丢弃 padding0（认证阶段填充）
	paddingLenBytes, err := b.ReadBytes(2)
	if err != nil {
		b.Resize(0, n)
		fallback(ctx, c)
		return
	}
	paddingLen := binary.BigEndian.Uint16(paddingLenBytes)
	if paddingLen > 0 {
		if _, err = b.ReadBytes(int(paddingLen)); err != nil {
			b.Resize(0, n)
			fallback(ctx, c)
			return
		}
	}

	// 认证成功，查找用户 ID 并记录（用于流量统计）
	userID, _ := s.authenticate(passwordHashBytes)

	// 建立会话层，在每个新 Stream 上执行代理逻辑
	sess := session.NewServerSession(c, func(stream *session.Stream) {
		defer func() {
			if r := recover(); r != nil {
				logrus.Errorln("[BUG]", r, string(debug.Stack()))
			}
		}()
		defer stream.Close()

		// 解析代理目标地址（SocksAddr 格式）
		destination, err := M.SocksaddrSerializer.ReadAddrPort(stream)
		if err != nil {
			logrus.Debugln("ReadAddrPort:", err)
			return
		}

		var upload, download int64
		if strings.Contains(destination.String(), "udp-over-tcp.arpa") {
			upload, download = proxyOutboundUoT(ctx, stream, destination)
		} else {
			upload, download = proxyOutboundTCP(ctx, stream, destination)
		}

		// 记录本次代理的流量
		s.recordTraffic(userID, upload, download)
	}, &padding.DefaultPaddingFactory)

	sess.Run()
	sess.Close()
}

// isValidAuth 验证认证哈希并在失败时执行 fallback，避免重复代码
func isValidAuth(passwordHashBytes []byte, s *myServer, n int, c net.Conn, b *buf.Buffer) bool {
	ctx := context.Background()
	_, ok := s.authenticate(passwordHashBytes)
	if !ok {
		b.Resize(0, n)
		fallback(ctx, c)
		return false
	}
	return true
}

// fallback 处理认证失败的连接（当前为简单关闭，可扩展为 HTTP fallback）
func fallback(ctx context.Context, c net.Conn) {
	// 暂未实现 HTTP fallback
	logrus.Debugln("fallback:", c.RemoteAddr())
}
