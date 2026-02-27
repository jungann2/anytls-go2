package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func newTestLogger() *logrus.Logger {
	l := logrus.New()
	l.SetLevel(logrus.DebugLevel)
	l.SetOutput(io.Discard)
	return l
}

// TestFetchConfig verifies that FetchConfig correctly parses a valid config JSON response.
func TestFetchConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/server/UniProxy/config" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// Verify query params
		q := r.URL.Query()
		if q.Get("token") != "test-token" {
			t.Errorf("unexpected token: %s", q.Get("token"))
		}
		if q.Get("node_id") != "42" {
			t.Errorf("unexpected node_id: %s", q.Get("node_id"))
		}
		if q.Get("node_type") != "anytls" {
			t.Errorf("unexpected node_type: %s", q.Get("node_type"))
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"server_port":    8443,
			"server_name":    "example.com",
			"padding_scheme": []string{"stop=8", "0=30-30", "1=100-400"},
			"base_config": map[string]int{
				"push_interval": 60,
				"pull_interval": 30,
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	cfg, err := client.FetchConfig()
	if err != nil {
		t.Fatalf("FetchConfig failed: %v", err)
	}

	if cfg.ServerPort != 8443 {
		t.Errorf("ServerPort = %d, want 8443", cfg.ServerPort)
	}
	if cfg.ServerName != "example.com" {
		t.Errorf("ServerName = %q, want %q", cfg.ServerName, "example.com")
	}
	if len(cfg.PaddingScheme) != 3 {
		t.Errorf("PaddingScheme length = %d, want 3", len(cfg.PaddingScheme))
	}
	if cfg.BaseConfig.PushInterval != 60 {
		t.Errorf("PushInterval = %d, want 60", cfg.BaseConfig.PushInterval)
	}
	if cfg.BaseConfig.PullInterval != 30 {
		t.Errorf("PullInterval = %d, want 30", cfg.BaseConfig.PullInterval)
	}
}

// TestFetchUsers verifies that FetchUsers correctly parses a user list JSON response.
func TestFetchUsers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/user" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		speedLimit := 100
		deviceLimit := 3
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"etag-abc123"`)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": []map[string]interface{}{
				{"id": 1, "uuid": "550e8400-e29b-41d4-a716-446655440000", "speed_limit": speedLimit, "device_limit": deviceLimit},
				{"id": 2, "uuid": "660e8400-e29b-41d4-a716-446655440001", "speed_limit": nil, "device_limit": nil},
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	users, err := client.FetchUsers()
	if err != nil {
		t.Fatalf("FetchUsers failed: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	// User 1: has limits
	if users[0].ID != 1 {
		t.Errorf("user[0].ID = %d, want 1", users[0].ID)
	}
	if users[0].UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("user[0].UUID = %q, want %q", users[0].UUID, "550e8400-e29b-41d4-a716-446655440000")
	}
	if users[0].SpeedLimit == nil || *users[0].SpeedLimit != 100 {
		t.Errorf("user[0].SpeedLimit = %v, want 100", users[0].SpeedLimit)
	}
	if users[0].DeviceLimit == nil || *users[0].DeviceLimit != 3 {
		t.Errorf("user[0].DeviceLimit = %v, want 3", users[0].DeviceLimit)
	}

	// User 2: nil limits
	if users[1].SpeedLimit != nil {
		t.Errorf("user[1].SpeedLimit = %v, want nil", users[1].SpeedLimit)
	}
	if users[1].DeviceLimit != nil {
		t.Errorf("user[1].DeviceLimit = %v, want nil", users[1].DeviceLimit)
	}

	// Verify ETag was stored
	if client.userETag != `"etag-abc123"` {
		t.Errorf("userETag = %q, want %q", client.userETag, `"etag-abc123"`)
	}
}

// TestFetchUsers_ETag verifies that the second call sends If-None-Match with the stored ETag.
func TestFetchUsers_ETag(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// First call: return users + ETag
			w.Header().Set("ETag", `"etag-first"`)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []map[string]interface{}{
					{"id": 1, "uuid": "test-uuid", "speed_limit": nil, "device_limit": nil},
				},
			})
			return
		}
		// Second call: verify If-None-Match header and return 304
		inm := r.Header.Get("If-None-Match")
		if inm != `"etag-first"` {
			t.Errorf("If-None-Match = %q, want %q", inm, `"etag-first"`)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())

	// First call
	users, err := client.FetchUsers()
	if err != nil {
		t.Fatalf("first FetchUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}

	// Second call — should get 304
	users, err = client.FetchUsers()
	if err != nil {
		t.Fatalf("second FetchUsers failed: %v", err)
	}
	if users != nil {
		t.Errorf("expected nil users on 304, got %v", users)
	}

	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

// TestFetchUsers_304 verifies that a 304 response returns nil users.
func TestFetchUsers_304(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	client.userETag = `"some-etag"`

	users, err := client.FetchUsers()
	if err != nil {
		t.Fatalf("FetchUsers failed: %v", err)
	}
	if users != nil {
		t.Errorf("expected nil users on 304, got %v", users)
	}
}

// TestPushTraffic verifies the POST body format for traffic push.
func TestPushTraffic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/server/UniProxy/push" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		body, _ := io.ReadAll(r.Body)
		var payload map[string][2]int64
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to parse push body: %v", err)
		}

		// Verify format: {"1": [upload, download]}
		traffic, ok := payload["1"]
		if !ok {
			t.Fatal("expected key '1' in payload")
		}
		if traffic[0] != 1024 || traffic[1] != 2048 {
			t.Errorf("traffic = %v, want [1024, 2048]", traffic)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	data := map[int][2]int64{
		1: {1024, 2048},
	}
	if err := client.PushTraffic(data); err != nil {
		t.Fatalf("PushTraffic failed: %v", err)
	}
}

// TestPushAlive verifies the POST body format for alive push.
func TestPushAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/server/UniProxy/alive" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		body, _ := io.ReadAll(r.Body)
		var payload map[string][]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("failed to parse alive body: %v", err)
		}

		ips, ok := payload["1"]
		if !ok {
			t.Fatal("expected key '1' in payload")
		}
		if len(ips) != 2 || ips[0] != "192.168.1.1_42" || ips[1] != "10.0.0.1_42" {
			t.Errorf("ips = %v, want [192.168.1.1_42, 10.0.0.1_42]", ips)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	data := map[int][]string{
		1: {"192.168.1.1_42", "10.0.0.1_42"},
	}
	if err := client.PushAlive(data); err != nil {
		t.Fatalf("PushAlive failed: %v", err)
	}
}

// TestPushStatus verifies the POST body format for status push.
func TestPushStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/server/UniProxy/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		body, _ := io.ReadAll(r.Body)
		var status NodeStatus
		if err := json.Unmarshal(body, &status); err != nil {
			t.Fatalf("failed to parse status body: %v", err)
		}

		if status.CPU != 25.5 {
			t.Errorf("CPU = %f, want 25.5", status.CPU)
		}
		if status.Mem.Total != 8589934592 || status.Mem.Used != 4294967296 {
			t.Errorf("Mem = %+v, unexpected", status.Mem)
		}
		if status.Disk.Total != 107374182400 || status.Disk.Used != 53687091200 {
			t.Errorf("Disk = %+v, unexpected", status.Disk)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	status := &NodeStatus{
		CPU:  25.5,
		Mem:  ResourceUsage{Total: 8589934592, Used: 4294967296},
		Swap: ResourceUsage{Total: 2147483648, Used: 0},
		Disk: ResourceUsage{Total: 107374182400, Used: 53687091200},
	}
	if err := client.PushStatus(status); err != nil {
		t.Fatalf("PushStatus failed: %v", err)
	}
}

// TestFetchAliveList verifies parsing of the alivelist response.
func TestFetchAliveList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/server/UniProxy/alivelist" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alive": map[string]int{
				"1": 2,
				"2": 1,
			},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	aliveMap, err := client.FetchAliveList()
	if err != nil {
		t.Fatalf("FetchAliveList failed: %v", err)
	}

	if len(aliveMap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(aliveMap))
	}
	if aliveMap[1] != 2 {
		t.Errorf("aliveMap[1] = %d, want 2", aliveMap[1])
	}
	if aliveMap[2] != 1 {
		t.Errorf("aliveMap[2] = %d, want 1", aliveMap[2])
	}
}

// TestRetry_ServerError verifies that 5xx errors trigger retries and eventually succeed.
func TestRetry_ServerError(t *testing.T) {
	// Override retry delays to speed up the test
	origDelays := retryDelays
	retryDelays = []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}
	defer func() { retryDelays = origDelays }()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Third call succeeds
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alive": map[string]int{"1": 1},
		})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())
	aliveMap, err := client.FetchAliveList()
	if err != nil {
		t.Fatalf("FetchAliveList failed after retries: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls (2 failures + 1 success), got %d", callCount)
	}
	if aliveMap[1] != 1 {
		t.Errorf("aliveMap[1] = %d, want 1", aliveMap[1])
	}
}

// TestRetry_ClientError verifies that 4xx errors do NOT trigger retries.
func TestRetry_ClientError(t *testing.T) {
	// Override retry delays to speed up the test
	origDelays := retryDelays
	retryDelays = []time.Duration{1 * time.Millisecond, 1 * time.Millisecond, 1 * time.Millisecond}
	defer func() { retryDelays = origDelays }()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-token", 42, "anytls", newTestLogger())

	// PushTraffic with a 400 response — should not retry, should return error about status code
	data := map[int][2]int64{1: {100, 200}}
	err := client.PushTraffic(data)
	// PushTraffic checks resp.StatusCode != 200 and returns error
	if err == nil {
		t.Fatal("expected error for 400 response")
	}

	if callCount != 1 {
		t.Errorf("expected 1 call (no retry for 4xx), got %d", callCount)
	}
}
