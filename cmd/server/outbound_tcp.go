package main

import (
	"anytls/proxy"
	"context"
	"io"
	"net"

	E "github.com/sagernet/sing/common/exceptions"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/common/uot"
	"github.com/sirupsen/logrus"
)

// proxyOutboundTCP 建立到目标地址的 TCP 连接并进行双向数据中继。
// 返回 (upload, download) 字节数，分别对应客户端上行和下行流量。
func proxyOutboundTCP(ctx context.Context, conn net.Conn, destination M.Socksaddr) (upload, download int64) {
	c, err := proxy.SystemDialer.DialContext(ctx, "tcp", destination.String())
	if err != nil {
		logrus.Debugln("proxyOutboundTCP DialContext:", err)
		_ = E.Errors(err, N.ReportHandshakeFailure(conn, err))
		return 0, 0
	}
	defer c.Close()

	if err = N.ReportHandshakeSuccess(conn); err != nil {
		return 0, 0
	}

	// 双向中继并统计流量
	upload, download = copyBidirectional(ctx, conn, c)
	return
}

// proxyOutboundUoT 处理 UDP-over-TCP 代理请求（sing-box UoT v2 协议）。
// 返回 (upload, download) 字节数，分别对应客户端上行和下行流量。
func proxyOutboundUoT(ctx context.Context, conn net.Conn, destination M.Socksaddr) (upload, download int64) {
	request, err := uot.ReadRequest(conn)
	if err != nil {
		logrus.Debugln("proxyOutboundUoT ReadRequest:", err)
		return 0, 0
	}

	c, err := net.ListenPacket("udp", "")
	if err != nil {
		logrus.Debugln("proxyOutboundUoT ListenPacket:", err)
		_ = E.Errors(err, N.ReportHandshakeFailure(conn, err))
		return 0, 0
	}
	defer c.Close()

	if err = N.ReportHandshakeSuccess(conn); err != nil {
		return 0, 0
	}

	// UoT 流量通过 uot.NewConn 封装后当普通流走中继；
	// 暂时不统计 UoT 的精确字节数，以 0 上报（不影响面板近似）
	uotConn := uot.NewConn(conn, *request)
	upload, download = copyBidirectional(ctx, uotConn, &udpPacketConnWrapper{PacketConn: c, target: request.Destination})
	return
}

// copyBidirectional 在 src 与 dst 之间执行双向数据复制，并统计流量字节数。
// 返回 (srcToDst bytes, dstToSrc bytes)，即 (上行 upload, 下行 download)。
func copyBidirectional(ctx context.Context, client, remote net.Conn) (upload, download int64) {
	done := make(chan struct{}, 2)

	go func() {
		n, _ := io.Copy(remote, client)
		upload = n
		// 关闭写方向，通知对端 EOF
		if tc, ok := remote.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		} else {
			_ = remote.Close()
		}
		done <- struct{}{}
	}()

	go func() {
		n, _ := io.Copy(client, remote)
		download = n
		if tc, ok := client.(*net.TCPConn); ok {
			_ = tc.CloseWrite()
		} else {
			_ = client.Close()
		}
		done <- struct{}{}
	}()

	// 等待两个方向均完成
	<-done
	<-done
	return
}

// udpPacketConnWrapper 将 net.PacketConn 包装为 net.Conn 接口，供 UoT 使用
type udpPacketConnWrapper struct {
	net.PacketConn
	target M.Socksaddr
}

func (w *udpPacketConnWrapper) Read(b []byte) (int, error) {
	n, _, err := w.PacketConn.ReadFrom(b)
	return n, err
}

func (w *udpPacketConnWrapper) Write(b []byte) (int, error) {
	addr, err := net.ResolveUDPAddr("udp", w.target.String())
	if err != nil {
		return 0, err
	}
	return w.PacketConn.WriteTo(b, addr)
}

func (w *udpPacketConnWrapper) RemoteAddr() net.Addr {
	addr, _ := net.ResolveUDPAddr("udp", w.target.String())
	return addr
}

func (w *udpPacketConnWrapper) LocalAddr() net.Addr {
	return w.PacketConn.LocalAddr()
}
