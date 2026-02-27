package ratelimit

import (
	"sync"
	"time"
)

const (
	maxFailures   = 10              // 60 秒内最大失败次数
	failWindow    = 60 * time.Second // 失败计数窗口
	banDuration   = 5 * time.Minute  // 封禁时长
)

type failureRecord struct {
	count     int
	firstFail time.Time
	bannedAt  time.Time
}

// ConnRateLimiter 连接速率限制器（防暴力破解）
type ConnRateLimiter struct {
	mu       sync.Mutex
	failures map[string]*failureRecord
}

// NewConnRateLimiter 创建连接速率限制器
func NewConnRateLimiter() *ConnRateLimiter {
	return &ConnRateLimiter{
		failures: make(map[string]*failureRecord),
	}
}

// RecordFailure 记录认证失败
func (r *ConnRateLimiter) RecordFailure(ip string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	rec, ok := r.failures[ip]
	if !ok {
		r.failures[ip] = &failureRecord{
			count:     1,
			firstFail: now,
		}
		return
	}

	// 如果已被封禁，不再计数
	if !rec.bannedAt.IsZero() {
		return
	}

	// 窗口过期，重置
	if now.Sub(rec.firstFail) > failWindow {
		rec.count = 1
		rec.firstFail = now
		return
	}

	rec.count++
	if rec.count > maxFailures {
		rec.bannedAt = now
	}
}

// IsBanned 检查 IP 是否被封禁
func (r *ConnRateLimiter) IsBanned(ip string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, ok := r.failures[ip]
	if !ok {
		return false
	}

	if rec.bannedAt.IsZero() {
		return false
	}

	// 封禁已过期
	if time.Since(rec.bannedAt) > banDuration {
		delete(r.failures, ip)
		return false
	}

	return true
}

// Cleanup 清理过期记录
func (r *ConnRateLimiter) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	for ip, rec := range r.failures {
		if !rec.bannedAt.IsZero() {
			// 封禁已过期
			if now.Sub(rec.bannedAt) > banDuration {
				delete(r.failures, ip)
			}
		} else {
			// 失败窗口已过期
			if now.Sub(rec.firstFail) > failWindow {
				delete(r.failures, ip)
			}
		}
	}
}
