package ratelimit

import (
	"sync"

	"golang.org/x/time/rate"
)

const bytesPerMbit = 125000 // 1 Mbps = 125000 bytes/s
const burstMultiplier = 128 * 1024 // burst = speed_limit * 128KB

// SpeedLimiter 用户限速器
type SpeedLimiter struct {
	mu       sync.RWMutex
	limiters map[int]*rate.Limiter
}

// NewSpeedLimiter 创建用户限速器
func NewSpeedLimiter() *SpeedLimiter {
	return &SpeedLimiter{
		limiters: make(map[int]*rate.Limiter),
	}
}

// GetLimiter 获取用户限速器
// speedLimitMbps > 0 时返回对应 Limiter，= 0 时返回 nil（不限速）
func (s *SpeedLimiter) GetLimiter(userID int, speedLimitMbps int) *rate.Limiter {
	if speedLimitMbps <= 0 {
		return nil
	}

	s.mu.RLock()
	l, ok := s.limiters[userID]
	s.mu.RUnlock()
	if ok {
		return l
	}

	// 创建新的限速器
	bytesPerSec := rate.Limit(float64(speedLimitMbps) * float64(bytesPerMbit))
	burst := speedLimitMbps * burstMultiplier
	l = rate.NewLimiter(bytesPerSec, burst)

	s.mu.Lock()
	s.limiters[userID] = l
	s.mu.Unlock()

	return l
}

// UpdateLimit 更新用户限速参数
func (s *SpeedLimiter) UpdateLimit(userID int, speedLimitMbps int) {
	if speedLimitMbps <= 0 {
		s.RemoveUser(userID)
		return
	}

	bytesPerSec := rate.Limit(float64(speedLimitMbps) * float64(bytesPerMbit))
	burst := speedLimitMbps * burstMultiplier

	s.mu.Lock()
	defer s.mu.Unlock()

	if l, ok := s.limiters[userID]; ok {
		l.SetLimit(bytesPerSec)
		l.SetBurst(burst)
	} else {
		s.limiters[userID] = rate.NewLimiter(bytesPerSec, burst)
	}
}

// RemoveUser 移除用户限速器
func (s *SpeedLimiter) RemoveUser(userID int) {
	s.mu.Lock()
	delete(s.limiters, userID)
	s.mu.Unlock()
}
