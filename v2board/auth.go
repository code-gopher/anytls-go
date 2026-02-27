// Package v2board 用户认证管理器
// 维护内存中的用户表（key 为 sha256(uuid) 哈希），与 anytls 协议认证层无缝对接。
package v2board

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// userEntry 是内存用户表中的条目
type userEntry struct {
	user         User
	passwordHash [sha256.Size]byte // sha256(uuid)
}

// AuthManager 管理 V2board 用户列表，并提供认证接口
type AuthManager struct {
	client *Client

	// mu 保护 usersByHash 和 usersById
	mu          sync.RWMutex
	usersByHash map[[sha256.Size]byte]*userEntry // key: sha256(uuid)
	usersById   map[int]*userEntry               // key: user.ID
}

// NewAuthManager 创建认证管理器，但不启动自动刷新
func NewAuthManager(client *Client) *AuthManager {
	return &AuthManager{
		client:      client,
		usersByHash: make(map[[sha256.Size]byte]*userEntry),
		usersById:   make(map[int]*userEntry),
	}
}

// Start 立即执行一次用户列表拉取，然后以 pullInterval 为周期定期刷新。
// 该方法应在 goroutine 中调用。
func (m *AuthManager) Start(pullInterval time.Duration) {
	logrus.Infof("[V2board] 用户列表自动更新已启动，周期: %v", pullInterval)

	// 立即执行第一次
	if err := m.refresh(); err != nil {
		logrus.Errorf("[V2board] 首次拉取用户列表失败: %v", err)
	}

	ticker := time.NewTicker(pullInterval)
	defer ticker.Stop()

	for range ticker.C {
		if err := m.refresh(); err != nil {
			logrus.Errorf("[V2board] 刷新用户列表失败: %v", err)
		}
	}
}

// refresh 从 V2board API 拉取最新用户列表并更新内存表
func (m *AuthManager) refresh() error {
	users, err := m.client.GetUserList()
	if err != nil {
		return fmt.Errorf("拉取用户列表: %w", err)
	}

	// 构建新映射
	newByHash := make(map[[sha256.Size]byte]*userEntry, len(users))
	newById := make(map[int]*userEntry, len(users))
	for i := range users {
		u := &users[i]
		hash := sha256.Sum256([]byte(u.UUID))
		entry := &userEntry{
			user:         *u,
			passwordHash: hash,
		}
		newByHash[hash] = entry
		newById[u.ID] = entry
	}

	m.mu.Lock()
	m.usersByHash = newByHash
	m.usersById = newById
	m.mu.Unlock()

	logrus.Debugf("[V2board] 用户列表已更新，共 %d 个用户", len(users))
	return nil
}

// CheckAuth 验证客户端发来的密码哈希是否匹配某个合法用户。
// passwordHash 是客户端在 TLS 握手后发来的 32 字节（sha256(uuid)）。
// 返回匹配的用户 ID 和是否认证成功。
func (m *AuthManager) CheckAuth(passwordHash []byte) (userID int, ok bool) {
	if len(passwordHash) != sha256.Size {
		return 0, false
	}

	var key [sha256.Size]byte
	copy(key[:], passwordHash)

	m.mu.RLock()
	entry, exists := m.usersByHash[key]
	m.mu.RUnlock()

	if !exists {
		return 0, false
	}
	return entry.user.ID, true
}

// GetUserByID 按用户 ID 查询用户信息（流量上报时使用）
func (m *AuthManager) GetUserByID(userID int) (*User, bool) {
	m.mu.RLock()
	entry, exists := m.usersById[userID]
	m.mu.RUnlock()

	if !exists {
		return nil, false
	}
	u := entry.user
	return &u, true
}

// UserCount 返回当前缓存的用户数量
func (m *AuthManager) UserCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.usersByHash)
}
