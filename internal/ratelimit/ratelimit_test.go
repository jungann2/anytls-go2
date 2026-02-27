package ratelimit

import (
	"fmt"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: anytls-xboard-integration, Property 6: 连接速率限制
// **Validates: Requirements 8.1**

func TestProperty6_ConnRateLimiter(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Sub-property: failures <= 10 should NOT ban
	properties.Property("IP with failures <= maxFailures is not banned", prop.ForAll(
		func(ipSuffix int, failCount int) bool {
			limiter := NewConnRateLimiter()
			ip := fmt.Sprintf("10.0.0.%d", ipSuffix%256)

			for i := 0; i < failCount; i++ {
				limiter.RecordFailure(ip)
			}

			// With failCount <= 10, IsBanned should return false
			return !limiter.IsBanned(ip)
		},
		gen.IntRange(1, 255),
		gen.IntRange(0, 10), // 0 to maxFailures (10)
	))

	// Sub-property: failures > 10 should ban
	properties.Property("IP with failures > maxFailures is banned", prop.ForAll(
		func(ipSuffix int, extraFailures int) bool {
			limiter := NewConnRateLimiter()
			ip := fmt.Sprintf("10.0.0.%d", ipSuffix%256)

			// Record 11 + extraFailures failures (always > 10)
			totalFailures := 11 + extraFailures
			for i := 0; i < totalFailures; i++ {
				limiter.RecordFailure(ip)
			}

			// Should be banned
			return limiter.IsBanned(ip)
		},
		gen.IntRange(1, 255),
		gen.IntRange(0, 20),
	))

	// Sub-property: exact threshold - 10 failures NOT banned, 11 bans
	properties.Property("Exact threshold: 10 failures not banned, 11 bans", prop.ForAll(
		func(ipSuffix int) bool {
			limiter := NewConnRateLimiter()
			ip := fmt.Sprintf("192.168.1.%d", ipSuffix%256)

			// Record exactly 10 failures
			for i := 0; i < 10; i++ {
				limiter.RecordFailure(ip)
			}
			if limiter.IsBanned(ip) {
				return false // 10 failures should NOT ban
			}

			// Record one more (11th)
			limiter.RecordFailure(ip)
			return limiter.IsBanned(ip) // 11 failures SHOULD ban
		},
		gen.IntRange(1, 255),
	))

	// Sub-property: different IPs are independent
	properties.Property("Different IPs have independent failure counts", prop.ForAll(
		func(ipA int, ipB int) bool {
			if ipA == ipB {
				return true // skip when same IP
			}
			limiter := NewConnRateLimiter()
			ip1 := fmt.Sprintf("10.1.0.%d", ipA%256)
			ip2 := fmt.Sprintf("10.2.0.%d", ipB%256)

			// Ban ip1 with 11 failures
			for i := 0; i < 11; i++ {
				limiter.RecordFailure(ip1)
			}

			// ip2 should not be affected
			return limiter.IsBanned(ip1) && !limiter.IsBanned(ip2)
		},
		gen.IntRange(1, 255),
		gen.IntRange(1, 255),
	))

	properties.TestingRun(t)
}

// Feature: anytls-xboard-integration, Property 9: 限速令牌桶行为
// **Validates: Requirements 4.1, 4.2**

func TestProperty9_SpeedLimiterTokenBucket(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Sub-property: speed_limit = 0 returns nil (no limit)
	properties.Property("GetLimiter returns nil when speedLimit is 0", prop.ForAll(
		func(userID int) bool {
			sl := NewSpeedLimiter()
			limiter := sl.GetLimiter(userID, 0)
			return limiter == nil
		},
		gen.IntRange(1, 1000),
	))

	// Sub-property: speed_limit > 0 returns non-nil limiter
	properties.Property("GetLimiter returns non-nil when speedLimit > 0", prop.ForAll(
		func(userID int, speedLimit int) bool {
			sl := NewSpeedLimiter()
			limiter := sl.GetLimiter(userID, speedLimit)
			return limiter != nil
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 100),
	))

	// Sub-property: limiter rate matches speed_limit
	properties.Property("Limiter rate approximately equals speedLimit * bytesPerMbit", prop.ForAll(
		func(userID int, speedLimit int) bool {
			sl := NewSpeedLimiter()
			limiter := sl.GetLimiter(userID, speedLimit)
			if limiter == nil {
				return false
			}

			expectedRate := float64(speedLimit) * float64(bytesPerMbit)
			actualRate := float64(limiter.Limit())

			// Allow small floating point tolerance
			ratio := actualRate / expectedRate
			return ratio > 0.99 && ratio < 1.01
		},
		gen.IntRange(1, 100),
		gen.IntRange(1, 100),
	))

	// Sub-property: limiter burst matches speed_limit * burstMultiplier
	properties.Property("Limiter burst equals speedLimit * burstMultiplier", prop.ForAll(
		func(userID int, speedLimit int) bool {
			sl := NewSpeedLimiter()
			limiter := sl.GetLimiter(userID, speedLimit)
			if limiter == nil {
				return false
			}

			expectedBurst := speedLimit * burstMultiplier
			return limiter.Burst() == expectedBurst
		},
		gen.IntRange(1, 100),
		gen.IntRange(1, 100),
	))

	// Sub-property: same userID returns same limiter instance
	properties.Property("Same userID returns same limiter on repeated calls", prop.ForAll(
		func(userID int, speedLimit int) bool {
			sl := NewSpeedLimiter()
			l1 := sl.GetLimiter(userID, speedLimit)
			l2 := sl.GetLimiter(userID, speedLimit)
			return l1 == l2
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 100),
	))

	// Sub-property: negative speed_limit returns nil
	properties.Property("GetLimiter returns nil for negative speedLimit", prop.ForAll(
		func(userID int, speedLimit int) bool {
			sl := NewSpeedLimiter()
			limiter := sl.GetLimiter(userID, speedLimit)
			return limiter == nil
		},
		gen.IntRange(1, 1000),
		gen.IntRange(-100, -1),
	))

	properties.TestingRun(t)
}
