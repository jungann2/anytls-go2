package user

import (
	"crypto/sha256"
	"sync/atomic"

	"anytls/internal/api"
)

// UserEntry 用户条目
type UserEntry struct {
	ID           int
	UUID         string
	SpeedLimit   int // Mbps, 0=不限
	DeviceLimit  int // 0=不限
	PasswordHash [32]byte
}

// UserTable 用户表（不可变，原子替换）
type UserTable struct {
	byPasswordHash map[[32]byte]*UserEntry
	byID           map[int]*UserEntry
}

// Manager 用户管理器
type Manager struct {
	users atomic.Value // *UserTable
}

// NewManager 创建用户管理器
func NewManager() *Manager {
	m := &Manager{}
	m.users.Store(&UserTable{
		byPasswordHash: make(map[[32]byte]*UserEntry),
		byID:           make(map[int]*UserEntry),
	})
	return m
}

// Authenticate 根据密码哈希查找用户
// passwordHash 为客户端发送的 SHA256(uuid) 的 32 字节哈希
func (m *Manager) Authenticate(passwordHash []byte) *UserEntry {
	if len(passwordHash) != 32 {
		return nil
	}
	table := m.users.Load().(*UserTable)
	var hash [32]byte
	copy(hash[:], passwordHash)
	return table.byPasswordHash[hash]
}

// UpdateUsers 原子替换用户表
// 从 API 用户列表构建新 UserTable，预计算 SHA256 哈希
func (m *Manager) UpdateUsers(users []api.User) {
	table := &UserTable{
		byPasswordHash: make(map[[32]byte]*UserEntry, len(users)),
		byID:           make(map[int]*UserEntry, len(users)),
	}

	for _, u := range users {
		hash := sha256.Sum256([]byte(u.UUID))
		entry := &UserEntry{
			ID:           u.ID,
			UUID:         u.UUID,
			PasswordHash: hash,
		}
		if u.SpeedLimit != nil {
			entry.SpeedLimit = *u.SpeedLimit
		}
		if u.DeviceLimit != nil {
			entry.DeviceLimit = *u.DeviceLimit
		}
		table.byPasswordHash[hash] = entry
		table.byID[u.ID] = entry
	}

	m.users.Store(table)
}

// GetUser 根据 ID 获取用户
func (m *Manager) GetUser(id int) *UserEntry {
	table := m.users.Load().(*UserTable)
	return table.byID[id]
}

// GetUserCount 获取当前用户数量
func (m *Manager) GetUserCount() int {
	table := m.users.Load().(*UserTable)
	return len(table.byID)
}
