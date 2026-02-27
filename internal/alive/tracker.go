package alive

import (
	"fmt"
	"sync"
	"time"
)

// Tracker 在线设备追踪器
type Tracker struct {
	mu     sync.RWMutex
	online map[int]map[string]time.Time // user_id -> {ip -> last_seen}
	nodeID int                          // 当前节点 ID，用于生成 "ip_nodeId" 格式
}

// NewTracker 创建在线设备追踪器
func NewTracker(nodeID int) *Tracker {
	return &Tracker{
		online: make(map[int]map[string]time.Time),
		nodeID: nodeID,
	}
}

// Track 记录用户在线
func (t *Tracker) Track(userID int, ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.online[userID] == nil {
		t.online[userID] = make(map[string]time.Time)
	}
	t.online[userID][ip] = time.Now()
}

// Remove 移除用户连接
func (t *Tracker) Remove(userID int, ip string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	ips := t.online[userID]
	if ips == nil {
		return
	}
	delete(ips, ip)
	if len(ips) == 0 {
		delete(t.online, userID)
	}
}

// Snapshot 获取当前在线用户快照
// 返回格式: map[user_id][]string，IP 已附加 "_{nodeID}" 后缀
// 如: {1: ["192.168.1.1_42", "10.0.0.1_42"]}
func (t *Tracker) Snapshot() map[int][]string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[int][]string, len(t.online))
	suffix := fmt.Sprintf("_%d", t.nodeID)
	for userID, ips := range t.online {
		list := make([]string, 0, len(ips))
		for ip := range ips {
			list = append(list, ip+suffix)
		}
		result[userID] = list
	}
	return result
}

// CheckDeviceLimit 检查设备限制
// aliveList 为从 API 获取的全局在线设备数量 map[user_id]count
// 当 deviceLimit=0 直接返回 true（不限制）
// 当全局在线数 >= deviceLimit 时返回 false（拒绝连接）
func (t *Tracker) CheckDeviceLimit(userID int, deviceLimit int, aliveList map[int]int) bool {
	if deviceLimit == 0 {
		return true
	}
	if aliveList[userID] >= deviceLimit {
		return false
	}
	return true
}
