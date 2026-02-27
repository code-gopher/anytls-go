// Package v2board 流量统计与定时上报
// 追踪每个用户的上下行字节数，并以可配置周期批量上报给 V2board 面板。
package v2board

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/sirupsen/logrus"
)

// userTraffic 保存单个用户的流量计数器（原子操作，无锁读写）
type userTraffic struct {
	upload   atomic.Int64
	download atomic.Int64
}

// TrafficManager 负责统计各用户流量并定期上报
type TrafficManager struct {
	client *Client

	// mu 保护 counters map 结构本身（增删），原子计数器内部无需加锁
	mu       sync.RWMutex
	counters map[int]*userTraffic // key: userID
}

// NewTrafficManager 创建流量统计管理器
func NewTrafficManager(client *Client) *TrafficManager {
	return &TrafficManager{
		client:   client,
		counters: make(map[int]*userTraffic),
	}
}

// Record 记录一次代理连接的流量（线程安全）
// userID: V2board 中的用户数字 ID
// upload: 本次连接客户端上行字节数（client -> server -> target）
// download: 本次连接客户端下行字节数（target -> server -> client）
func (m *TrafficManager) Record(userID int, upload, download int64) {
	if upload <= 0 && download <= 0 {
		return
	}

	m.mu.RLock()
	counter, exists := m.counters[userID]
	m.mu.RUnlock()

	if !exists {
		// 双检锁，避免重复初始化
		m.mu.Lock()
		if counter, exists = m.counters[userID]; !exists {
			counter = &userTraffic{}
			m.counters[userID] = counter
		}
		m.mu.Unlock()
	}

	if upload > 0 {
		counter.upload.Add(upload)
	}
	if download > 0 {
		counter.download.Add(download)
	}
}

// Start 立即执行一次上报，然后以 pushInterval 为周期定期上报。
// 该方法应在 goroutine 中调用。
func (m *TrafficManager) Start(pushInterval time.Duration) {
	logrus.Infof("[V2board] 流量上报服务已启动，周期: %v", pushInterval)

	ticker := time.NewTicker(pushInterval)
	defer ticker.Stop()

	for range ticker.C {
		m.push()
	}
}

// push 收集当前所有用户的流量数据，上报后清零计数器
func (m *TrafficManager) push() {
	// 收集快照并清零
	// 注意：先 Swap 再上报，防止上报失败时丢失数据；
	// 这里选择简单策略：上报失败时本轮数据丢弃，避免重复计费。
	var records []TrafficRecord

	m.mu.RLock()
	for userID, counter := range m.counters {
		upload := counter.upload.Swap(0)
		download := counter.download.Swap(0)
		if upload > 0 || download > 0 {
			records = append(records, TrafficRecord{
				UserID:   userID,
				Upload:   upload,
				Download: download,
			})
		}
	}
	m.mu.RUnlock()

	if len(records) == 0 {
		return
	}

	if err := m.client.PushTraffic(records); err != nil {
		logrus.Errorf("[V2board] 流量上报失败: %v", err)
		// 上报失败时将数据退还，避免丢失
		m.mu.RLock()
		for _, rec := range records {
			if counter, exists := m.counters[rec.UserID]; exists {
				counter.upload.Add(rec.Upload)
				counter.download.Add(rec.Download)
			}
		}
		m.mu.RUnlock()
		return
	}

	logrus.Infof("[V2board] 流量上报成功，共 %d 个用户", len(records))
}
