package main

import (
	"anytls/v2board"
	"crypto/tls"
)

// myServer 代表服务器实例，支持两种鉴权模式：
//   - 普通密码模式：使用固定的 sha256(password)
//   - V2board 模式：从面板动态拉取用户列表，使用 sha256(uuid) 认证
type myServer struct {
	tlsConfig *tls.Config

	// 普通密码模式（与 V2board 模式互斥）
	passwordSha256 []byte

	// V2board 模式（与普通密码模式互斥）
	v2boardAuth    *v2board.AuthManager
	v2boardTraffic *v2board.TrafficManager
}

// NewMyServer 创建普通密码模式的服务器实例
func NewMyServer(tlsConfig *tls.Config, passwordSha256 []byte) *myServer {
	return &myServer{
		tlsConfig:      tlsConfig,
		passwordSha256: passwordSha256,
	}
}

// NewMyServerV2board 创建 V2board 模式的服务器实例
func NewMyServerV2board(tlsConfig *tls.Config, authMgr *v2board.AuthManager, trafficMgr *v2board.TrafficManager) *myServer {
	return &myServer{
		tlsConfig:      tlsConfig,
		v2boardAuth:    authMgr,
		v2boardTraffic: trafficMgr,
	}
}

// authenticate 验证客户端发来的 32 字节密码哈希。
// 返回该用户的 ID（V2board 模式下为真实用户 ID；普通模式下固定返回 0）和认证结果。
func (s *myServer) authenticate(passwordHash []byte) (userID int, ok bool) {
	if s.v2boardAuth != nil {
		// V2board 模式：在用户表中查找 sha256(uuid)
		return s.v2boardAuth.CheckAuth(passwordHash)
	}
	// 普通密码模式：对比固定哈希
	if len(passwordHash) == len(s.passwordSha256) {
		match := true
		for i := range passwordHash {
			if passwordHash[i] != s.passwordSha256[i] {
				match = false
				break
			}
		}
		if match {
			return 0, true
		}
	}
	return 0, false
}

// recordTraffic 在连接结束后记录该用户的流量（仅 V2board 模式有效）
func (s *myServer) recordTraffic(userID int, upload, download int64) {
	if s.v2boardTraffic != nil && userID > 0 {
		s.v2boardTraffic.Record(userID, upload, download)
	}
}
