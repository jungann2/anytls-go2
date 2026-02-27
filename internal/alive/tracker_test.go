package alive

import (
	"fmt"
	"sort"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: anytls-xboard-integration, Property 5: 在线设备追踪与限制
// **Validates: Requirements 5.1, 5.3**

func TestProperty5_AliveTracker(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	// Sub-property: Track then Snapshot contains all tracked IPs with nodeID suffix
	properties.Property("Snapshot contains all tracked IPs with nodeID suffix", prop.ForAll(
		func(nodeID int, userID int, ipCount int) bool {
			tracker := NewTracker(nodeID)
			suffix := fmt.Sprintf("_%d", nodeID)

			ips := make([]string, ipCount)
			for i := range ips {
				ips[i] = fmt.Sprintf("10.0.%d.%d", i/256, i%256)
				tracker.Track(userID, ips[i])
			}

			snap := tracker.Snapshot()
			snapIPs := snap[userID]
			if len(snapIPs) != ipCount {
				return false
			}

			// Verify all IPs present with suffix
			sort.Strings(snapIPs)
			expected := make([]string, ipCount)
			for i, ip := range ips {
				expected[i] = ip + suffix
			}
			sort.Strings(expected)

			for i := range expected {
				if snapIPs[i] != expected[i] {
					return false
				}
			}
			return true
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 10000),
		gen.IntRange(1, 10),
	))

	// Sub-property: Remove then Snapshot does not contain removed IP
	properties.Property("Removed IPs are not in Snapshot", prop.ForAll(
		func(nodeID int, userID int) bool {
			tracker := NewTracker(nodeID)
			tracker.Track(userID, "1.2.3.4")
			tracker.Track(userID, "5.6.7.8")

			tracker.Remove(userID, "1.2.3.4")

			snap := tracker.Snapshot()
			for _, ip := range snap[userID] {
				if ip == fmt.Sprintf("1.2.3.4_%d", nodeID) {
					return false
				}
			}
			// "5.6.7.8" should still be there
			found := false
			for _, ip := range snap[userID] {
				if ip == fmt.Sprintf("5.6.7.8_%d", nodeID) {
					found = true
				}
			}
			return found
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 10000),
	))

	// Sub-property: Remove all IPs for a user removes the user from snapshot
	properties.Property("Removing all IPs removes user from snapshot", prop.ForAll(
		func(nodeID int, userID int, ipCount int) bool {
			tracker := NewTracker(nodeID)
			ips := make([]string, ipCount)
			for i := range ips {
				ips[i] = fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
				tracker.Track(userID, ips[i])
			}

			for _, ip := range ips {
				tracker.Remove(userID, ip)
			}

			snap := tracker.Snapshot()
			_, exists := snap[userID]
			return !exists
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 10000),
		gen.IntRange(1, 10),
	))

	// Sub-property: CheckDeviceLimit with device_limit=0 always returns true
	properties.Property("CheckDeviceLimit returns true when deviceLimit=0", prop.ForAll(
		func(userID int, aliveCount int) bool {
			tracker := NewTracker(1)
			aliveList := map[int]int{userID: aliveCount}
			return tracker.CheckDeviceLimit(userID, 0, aliveList)
		},
		gen.IntRange(1, 10000),
		gen.IntRange(0, 100),
	))

	// Sub-property: CheckDeviceLimit returns false when alive >= limit (limit > 0)
	properties.Property("CheckDeviceLimit returns false when alive >= deviceLimit > 0", prop.ForAll(
		func(userID int, deviceLimit int, extra int) bool {
			tracker := NewTracker(1)
			aliveCount := deviceLimit + extra // always >= deviceLimit
			aliveList := map[int]int{userID: aliveCount}
			return !tracker.CheckDeviceLimit(userID, deviceLimit, aliveList)
		},
		gen.IntRange(1, 10000),
		gen.IntRange(1, 50),
		gen.IntRange(0, 20),
	))

	// Sub-property: CheckDeviceLimit returns true when alive < limit
	properties.Property("CheckDeviceLimit returns true when alive < deviceLimit", prop.ForAll(
		func(userID int, deviceLimit int) bool {
			tracker := NewTracker(1)
			aliveCount := deviceLimit - 1 // always < deviceLimit
			aliveList := map[int]int{userID: aliveCount}
			return tracker.CheckDeviceLimit(userID, deviceLimit, aliveList)
		},
		gen.IntRange(1, 10000),
		gen.IntRange(2, 50), // min 2 so aliveCount >= 1
	))

	// Sub-property: different users are independent
	properties.Property("Different users have independent tracking", prop.ForAll(
		func(nodeID int, userA int, userB int) bool {
			if userA == userB {
				return true
			}
			tracker := NewTracker(nodeID)
			tracker.Track(userA, "1.1.1.1")
			tracker.Track(userB, "2.2.2.2")

			tracker.Remove(userA, "1.1.1.1")

			snap := tracker.Snapshot()
			// userA should be gone
			if _, exists := snap[userA]; exists {
				return false
			}
			// userB should still be there
			if len(snap[userB]) != 1 {
				return false
			}
			return true
		},
		gen.IntRange(1, 1000),
		gen.IntRange(1, 5000),
		gen.IntRange(5001, 10000),
	))

	properties.TestingRun(t)
}
