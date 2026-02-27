package traffic

import (
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// trafficTuple represents a single (userID, upload, download) operation.
type trafficTuple struct {
	UserID   int
	Upload   int64
	Download int64
}

// genTrafficTuple generates a random traffic tuple with userID in [1,50]
// and upload/download in [0, 10_000_000].
func genTrafficTuple() gopter.Gen {
	return gopter.CombineGens(
		gen.IntRange(1, 50),
		gen.Int64Range(0, 10_000_000),
		gen.Int64Range(0, 10_000_000),
	).Map(func(vals []interface{}) trafficTuple {
		return trafficTuple{
			UserID:   vals[0].(int),
			Upload:   vals[1].(int64),
			Download: vals[2].(int64),
		}
	})
}

// genTrafficTuples generates a slice of 1..100 traffic tuples.
func genTrafficTuples() gopter.Gen {
	return gen.SliceOfN(100, genTrafficTuple())
}

// Feature: anytls-xboard-integration, Property 3: 流量累加不变量
// **Validates: Requirements 3.1, 3.2**

func TestProperty3_TrafficAccumulationInvariant(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Concurrent Add totals equal sum of all inputs", prop.ForAll(
		func(tuples []trafficTuple) bool {
			c := NewCounter()

			// Compute expected sums
			expectedUp := make(map[int]int64)
			expectedDown := make(map[int]int64)
			for _, tt := range tuples {
				expectedUp[tt.UserID] += tt.Upload
				expectedDown[tt.UserID] += tt.Download
			}

			// Split tuples across multiple goroutines
			const numGoroutines = 4
			chunkSize := (len(tuples) + numGoroutines - 1) / numGoroutines

			var wg sync.WaitGroup
			for g := 0; g < numGoroutines; g++ {
				start := g * chunkSize
				end := start + chunkSize
				if end > len(tuples) {
					end = len(tuples)
				}
				if start >= len(tuples) {
					break
				}
				wg.Add(1)
				go func(chunk []trafficTuple) {
					defer wg.Done()
					for _, tt := range chunk {
						c.Add(tt.UserID, tt.Upload, tt.Download)
					}
				}(tuples[start:end])
			}
			wg.Wait()

			// Snapshot and verify
			snap := c.Snapshot()
			for uid, expUp := range expectedUp {
				expDown := expectedDown[uid]
				if expUp == 0 && expDown == 0 {
					continue
				}
				got, ok := snap[uid]
				if !ok {
					t.Logf("user %d missing from snapshot", uid)
					return false
				}
				if got[0] != expUp {
					t.Logf("user %d upload: got %d, want %d", uid, got[0], expUp)
					return false
				}
				if got[1] != expDown {
					t.Logf("user %d download: got %d, want %d", uid, got[1], expDown)
					return false
				}
			}

			// Verify no extra users in snapshot
			for uid := range snap {
				if expectedUp[uid] == 0 && expectedDown[uid] == 0 {
					t.Logf("unexpected user %d in snapshot", uid)
					return false
				}
			}

			return true
		},
		genTrafficTuples(),
	))

	properties.TestingRun(t)
}

// Feature: anytls-xboard-integration, Property 4: Snapshot 清零
// **Validates: Requirements 3.4**

func TestProperty4_SnapshotClearsData(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Snapshot returns correct data and second Snapshot returns empty", prop.ForAll(
		func(tuples []trafficTuple) bool {
			c := NewCounter()

			// Compute expected sums
			expectedUp := make(map[int]int64)
			expectedDown := make(map[int]int64)
			for _, tt := range tuples {
				expectedUp[tt.UserID] += tt.Upload
				expectedDown[tt.UserID] += tt.Download
				c.Add(tt.UserID, tt.Upload, tt.Download)
			}

			// First Snapshot should return all non-zero data
			snap1 := c.Snapshot()
			for uid, expUp := range expectedUp {
				expDown := expectedDown[uid]
				if expUp == 0 && expDown == 0 {
					continue
				}
				got, ok := snap1[uid]
				if !ok {
					t.Logf("first snapshot: user %d missing", uid)
					return false
				}
				if got[0] != expUp || got[1] != expDown {
					t.Logf("first snapshot: user %d got [%d,%d], want [%d,%d]",
						uid, got[0], got[1], expUp, expDown)
					return false
				}
			}

			// Second Snapshot should return empty map
			snap2 := c.Snapshot()
			if len(snap2) != 0 {
				t.Logf("second snapshot not empty: %v", snap2)
				return false
			}

			return true
		},
		genTrafficTuples(),
	))

	properties.TestingRun(t)
}

// Feature: anytls-xboard-integration, Property 8: 流量持久化 round-trip
// **Validates: Requirements 12.1, 12.2**

func TestProperty8_PersistenceRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("SaveToFile then LoadFromFile preserves traffic data", prop.ForAll(
		func(tuples []trafficTuple) bool {
			c1 := NewCounter()

			// Add traffic data
			expectedUp := make(map[int]int64)
			expectedDown := make(map[int]int64)
			for _, tt := range tuples {
				expectedUp[tt.UserID] += tt.Upload
				expectedDown[tt.UserID] += tt.Download
				c1.Add(tt.UserID, tt.Upload, tt.Download)
			}

			// Save to temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "traffic.json")

			if err := c1.SaveToFile(tmpFile); err != nil {
				t.Logf("SaveToFile error: %v", err)
				return false
			}

			// Check if all data was zero (file should not exist)
			allZero := true
			for uid := range expectedUp {
				if expectedUp[uid] != 0 || expectedDown[uid] != 0 {
					allZero = false
					break
				}
			}
			if allZero {
				// File should not exist when all data is zero
				if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
					t.Logf("file should not exist for all-zero data")
					return false
				}
				return true
			}

			// Load into a new counter
			c2 := NewCounter()
			if err := c2.LoadFromFile(tmpFile); err != nil {
				t.Logf("LoadFromFile error: %v", err)
				return false
			}

			// Snapshot the new counter and verify equivalence
			snap := c2.Snapshot()

			for uid, expUp := range expectedUp {
				expDown := expectedDown[uid]
				if expUp == 0 && expDown == 0 {
					continue
				}
				got, ok := snap[uid]
				if !ok {
					t.Logf("loaded data missing user %d", uid)
					return false
				}
				if got[0] != expUp || got[1] != expDown {
					t.Logf("loaded data user %d: got [%d,%d], want [%d,%d]",
						uid, got[0], got[1], expUp, expDown)
					return false
				}
			}

			// Verify no extra users
			for uid := range snap {
				if expectedUp[uid] == 0 && expectedDown[uid] == 0 {
					t.Logf("unexpected user %d in loaded data", uid)
					return false
				}
			}

			// Verify snapshot sizes match
			nonZeroCount := 0
			for uid := range expectedUp {
				if expectedUp[uid] != 0 || expectedDown[uid] != 0 {
					nonZeroCount++
				}
			}
			if len(snap) != nonZeroCount {
				t.Logf("snapshot size %d != expected %d", len(snap), nonZeroCount)
				return false
			}

			if !reflect.DeepEqual(len(snap), nonZeroCount) {
				return false
			}

			return true
		},
		genTrafficTuples(),
	))

	properties.TestingRun(t)
}
