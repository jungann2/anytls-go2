package user

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"anytls/internal/api"

	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
)

// Feature: anytls-xboard-integration, Property 1: 认证 round-trip
// **Validates: Requirements 1.2, 1.3, 1.4**

// genUUID generates a random UUID v4 string from 16 random bytes.
func genUUID() gopter.Gen {
	return gen.SliceOfN(16, gen.UInt8()).Map(func(b []byte) string {
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // variant 10
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	})
}

// genUserSlice generates a slice of 1..20 api.User with unique IDs and random UUIDs.
func genUserSlice() gopter.Gen {
	return gen.IntRange(1, 20).FlatMap(func(v interface{}) gopter.Gen {
		n := v.(int)
		gens := make([]gopter.Gen, n)
		for i := 0; i < n; i++ {
			gens[i] = genUUID()
		}
		return gopter.CombineGens(gens...).Map(func(vals []interface{}) []api.User {
			users := make([]api.User, len(vals))
			for i, v := range vals {
				users[i] = api.User{
					ID:   i + 1,
					UUID: v.(string),
				}
			}
			return users
		})
	}, reflect.TypeOf([]api.User{}))
}

func TestProperty1_AuthRoundTrip(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Authenticate returns correct user for SHA256(uuid) and nil for random hash", prop.ForAll(
		func(users []api.User) bool {
			m := NewManager()
			m.UpdateUsers(users)

			// For each user, SHA256(uuid) should authenticate to the correct ID
			for _, u := range users {
				hash := sha256.Sum256([]byte(u.UUID))
				entry := m.Authenticate(hash[:])
				if entry == nil {
					t.Logf("Authenticate returned nil for user %d (uuid=%s)", u.ID, u.UUID)
					return false
				}
				if entry.ID != u.ID {
					t.Logf("Authenticate returned ID=%d, want %d", entry.ID, u.ID)
					return false
				}
				if entry.UUID != u.UUID {
					t.Logf("Authenticate returned UUID=%s, want %s", entry.UUID, u.UUID)
					return false
				}
			}

			// Random 32-byte hashes (not matching any user) should return nil
			for i := 0; i < 5; i++ {
				var randomHash [32]byte
				if _, err := rand.Read(randomHash[:]); err != nil {
					t.Logf("rand.Read error: %v", err)
					return false
				}
				entry := m.Authenticate(randomHash[:])
				if entry != nil {
					// Verify it's not an accidental collision (astronomically unlikely)
					matched := false
					for _, u := range users {
						h := sha256.Sum256([]byte(u.UUID))
						if h == randomHash {
							matched = true
							break
						}
					}
					if !matched {
						t.Logf("Authenticate returned non-nil for random hash")
						return false
					}
				}
			}

			return true
		},
		genUserSlice(),
	))

	properties.TestingRun(t)
}

// Feature: anytls-xboard-integration, Property 2: UserTable 并发安全更新
// **Validates: Requirements 1.5**

func TestProperty2_ConcurrentSafety(t *testing.T) {
	parameters := gopter.DefaultTestParameters()
	parameters.MinSuccessfulTests = 100

	properties := gopter.NewProperties(parameters)

	properties.Property("Concurrent Authenticate + UpdateUsers never panics and returns valid results", prop.ForAll(
		func(users []api.User) bool {
			m := NewManager()
			m.UpdateUsers(users)

			// Build a set of valid user IDs for validation
			validIDs := make(map[int]bool, len(users))
			for _, u := range users {
				validIDs[u.ID] = true
			}

			// Prepare a second user list for concurrent UpdateUsers
			users2 := make([]api.User, len(users))
			for i, u := range users {
				users2[i] = api.User{
					ID:   u.ID + 1000,
					UUID: u.UUID + "-v2",
				}
			}
			validIDs2 := make(map[int]bool, len(users2))
			for _, u := range users2 {
				validIDs2[u.ID] = true
			}

			const numReaders = 8
			const readsPerGoroutine = 50

			var wg sync.WaitGroup
			errCh := make(chan string, numReaders*readsPerGoroutine)

			// Launch reader goroutines that call Authenticate concurrently
			for r := 0; r < numReaders; r++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < readsPerGoroutine; i++ {
						// Try authenticating with hashes from the original user list
						for _, u := range users {
							hash := sha256.Sum256([]byte(u.UUID))
							entry := m.Authenticate(hash[:])
							if entry != nil {
								// Must be a valid entry from either the old or new table
								if !validIDs[entry.ID] && !validIDs2[entry.ID] {
									errCh <- fmt.Sprintf("got invalid ID %d", entry.ID)
									return
								}
							}
							// nil is acceptable — the table may have been swapped
						}
					}
				}()
			}

			// Writer goroutine: swap user table multiple times
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 20; i++ {
					if i%2 == 0 {
						m.UpdateUsers(users2)
					} else {
						m.UpdateUsers(users)
					}
				}
			}()

			wg.Wait()
			close(errCh)

			for errMsg := range errCh {
				t.Logf("concurrent error: %s", errMsg)
				return false
			}

			return true
		},
		genUserSlice(),
	))

	properties.TestingRun(t)
}
