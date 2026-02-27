package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"anytls/internal/api"
	"anytls/internal/config"
)

// Feature: anytls-xboard-integration, 11.6 集成测试
// **Validates: Requirements 2.1, 2.2, 2.3, 8.3**

// mockXboard 模拟 Xboard API 服务器，记录所有请求
type mockXboard struct {
	mu             sync.Mutex
	configCalls    int
	userCalls      int
	pushCalls      int
	aliveCalls     int
	statusCalls    int
	aliveListCalls int
	lastTraffic    map[string][2]int64
	lastAlive      map[string][]string
	users          []api.User
}

func newMockXboard(users []api.User) *mockXboard {
	return &mockXboard{users: users}
}

func (m *mockXboard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	path := r.URL.Path
	switch {
	case path == "/api/v1/server/UniProxy/config" && r.Method == "GET":
		m.configCalls++
		json.NewEncoder(w).Encode(api.NodeConfig{
			ServerPort: 9443,
			BaseConfig: struct {
				PushInterval int `json:"push_interval"`
				PullInterval int `json:"pull_interval"`
			}{PushInterval: 60, PullInterval: 60},
		})

	case path == "/api/v1/server/UniProxy/user" && r.Method == "GET":
		m.userCalls++
		w.Header().Set("ETag", `"test-etag-123"`)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"users": m.users,
		})

	case path == "/api/v1/server/UniProxy/push" && r.Method == "POST":
		m.pushCalls++
		var data map[string][2]int64
		json.NewDecoder(r.Body).Decode(&data)
		m.lastTraffic = data
		w.WriteHeader(http.StatusOK)

	case path == "/api/v1/server/UniProxy/alive" && r.Method == "POST":
		m.aliveCalls++
		var data map[string][]string
		json.NewDecoder(r.Body).Decode(&data)
		m.lastAlive = data
		w.WriteHeader(http.StatusOK)

	case path == "/api/v1/server/UniProxy/alivelist" && r.Method == "GET":
		m.aliveListCalls++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"alive": map[string]int{},
		})

	case path == "/api/v1/server/UniProxy/status" && r.Method == "POST":
		m.statusCalls++
		w.WriteHeader(http.StatusOK)

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (m *mockXboard) getStats() (configCalls, userCalls, pushCalls, aliveCalls, statusCalls int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.configCalls, m.userCalls, m.pushCalls, m.aliveCalls, m.statusCalls
}

func (m *mockXboard) getLastTraffic() map[string][2]int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastTraffic
}

func (m *mockXboard) getLastAlive() map[string][]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastAlive
}

// TestIntegration_StartFetchUsersAndPush 测试完整流程：
// 启动 → 拉取配置 → 拉取用户 → 模拟流量 → 上报
func TestIntegration_StartFetchUsersAndPush(t *testing.T) {
	speedLimit := 0
	deviceLimit := 0
	mock := newMockXboard([]api.User{
		{ID: 1, UUID: "user-uuid-001", SpeedLimit: &speedLimit, DeviceLimit: &deviceLimit},
		{ID: 2, UUID: "user-uuid-002", SpeedLimit: &speedLimit, DeviceLimit: &deviceLimit},
	})
	ts := httptest.NewServer(mock)
	defer ts.Close()

	cfg := &config.Config{
		Listen:   "127.0.0.1:0",
		APIHost:  ts.URL,
		APIToken: "test-token",
		NodeID:   42,
		NodeType: "anytls",
		Log:      config.LogConfig{Level: "debug"},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// 1. 验证 FetchConfig 被调用
	nodeConfig, err := srv.apiClient.FetchConfig()
	if err != nil {
		t.Fatalf("FetchConfig failed: %v", err)
	}
	if nodeConfig.ServerPort != 9443 {
		t.Errorf("expected server_port=9443, got %d", nodeConfig.ServerPort)
	}

	// 2. 验证 FetchUsers 被调用并更新用户表
	users, err := srv.apiClient.FetchUsers()
	if err != nil {
		t.Fatalf("FetchUsers failed: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	srv.userManager.UpdateUsers(users)

	// 3. 验证认证工作
	entry := srv.userManager.GetUser(1)
	if entry == nil {
		t.Fatal("expected user 1 to exist")
	}
	authResult := srv.userManager.Authenticate(entry.PasswordHash[:])
	if authResult == nil || authResult.ID != 1 {
		t.Fatal("authentication failed for user 1")
	}

	// 4. 模拟流量并验证上报
	srv.trafficCounter.Add(1, 1024, 2048)
	srv.trafficCounter.Add(2, 512, 1024)

	snapshot := srv.trafficCounter.Snapshot()
	if len(snapshot) != 2 {
		t.Fatalf("expected 2 users in snapshot, got %d", len(snapshot))
	}

	err = srv.apiClient.PushTraffic(snapshot)
	if err != nil {
		t.Fatalf("PushTraffic failed: %v", err)
	}

	lastTraffic := mock.getLastTraffic()
	if lastTraffic == nil {
		t.Fatal("expected traffic data to be pushed")
	}
	if t1, ok := lastTraffic["1"]; !ok || t1[0] != 1024 || t1[1] != 2048 {
		t.Errorf("unexpected traffic for user 1: %v", lastTraffic["1"])
	}
	if t2, ok := lastTraffic["2"]; !ok || t2[0] != 512 || t2[1] != 1024 {
		t.Errorf("unexpected traffic for user 2: %v", lastTraffic["2"])
	}

	// 5. 验证在线追踪和上报
	srv.aliveTracker.Track(1, "10.0.0.1")
	srv.aliveTracker.Track(1, "10.0.0.2")
	srv.aliveTracker.Track(2, "10.0.0.3")

	aliveSnap := srv.aliveTracker.Snapshot()
	err = srv.apiClient.PushAlive(aliveSnap)
	if err != nil {
		t.Fatalf("PushAlive failed: %v", err)
	}

	lastAlive := mock.getLastAlive()
	if lastAlive == nil {
		t.Fatal("expected alive data to be pushed")
	}
	// User 1 should have 2 IPs with _42 suffix
	if ips, ok := lastAlive["1"]; !ok || len(ips) != 2 {
		t.Errorf("expected 2 IPs for user 1, got %v", lastAlive["1"])
	}

	// 6. 验证 API 调用次数
	configCalls, userCalls, pushCalls, aliveCalls, _ := mock.getStats()
	if configCalls < 1 {
		t.Error("expected at least 1 config call")
	}
	if userCalls < 1 {
		t.Error("expected at least 1 user call")
	}
	if pushCalls < 1 {
		t.Error("expected at least 1 push call")
	}
	if aliveCalls < 1 {
		t.Error("expected at least 1 alive call")
	}
}

// TestIntegration_GracefulShutdown 测试优雅关闭：
// 启动服务 → 添加流量 → 关闭 → 验证流量已上报
func TestIntegration_GracefulShutdown(t *testing.T) {
	speedLimit := 0
	deviceLimit := 0
	var pushReceived atomic.Bool
	var receivedTraffic sync.Map

	mock := newMockXboard([]api.User{
		{ID: 1, UUID: "shutdown-test-uuid", SpeedLimit: &speedLimit, DeviceLimit: &deviceLimit},
	})
	ts := httptest.NewServer(mock)
	defer ts.Close()

	// Override push handler to track received data
	customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/server/UniProxy/push" && r.Method == "POST" {
			var data map[string][2]int64
			json.NewDecoder(r.Body).Decode(&data)
			for k, v := range data {
				receivedTraffic.Store(k, v)
			}
			pushReceived.Store(true)
			w.WriteHeader(http.StatusOK)
			return
		}
		mock.ServeHTTP(w, r)
	})
	ts2 := httptest.NewServer(customHandler)
	defer ts2.Close()

	cfg := &config.Config{
		Listen:   "127.0.0.1:0",
		APIHost:  ts2.URL,
		APIToken: "test-token",
		NodeID:   99,
		NodeType: "anytls",
		Log:      config.LogConfig{Level: "debug"},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Set nodeConfig so Shutdown doesn't panic
	srv.nodeConfig = &api.NodeConfig{
		ServerPort: 9443,
		BaseConfig: struct {
			PushInterval int `json:"push_interval"`
			PullInterval int `json:"pull_interval"`
		}{PushInterval: 60, PullInterval: 60},
	}

	// Add traffic before shutdown
	srv.trafficCounter.Add(1, 5000, 10000)

	// Shutdown should push traffic
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = srv.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}

	if !pushReceived.Load() {
		t.Error("expected traffic to be pushed during shutdown")
	}

	// Verify the traffic data was correct
	val, ok := receivedTraffic.Load("1")
	if !ok {
		t.Fatal("expected traffic for user 1")
	}
	traffic := val.([2]int64)
	if traffic[0] != 5000 || traffic[1] != 10000 {
		t.Errorf("expected [5000, 10000], got %v", traffic)
	}
}

// TestIntegration_ETagCaching 测试 ETag 缓存：
// 第一次拉取返回用户列表 + ETag → 第二次拉取发送 If-None-Match → 返回 304
func TestIntegration_ETagCaching(t *testing.T) {
	var callCount atomic.Int32
	etag := `"etag-v1"`

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/server/UniProxy/user":
			callCount.Add(1)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etag)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"users": []api.User{{ID: 1, UUID: "etag-test-uuid"}},
			})
		case "/api/v1/server/UniProxy/config":
			json.NewEncoder(w).Encode(api.NodeConfig{ServerPort: 9443})
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	ts := httptest.NewServer(handler)
	defer ts.Close()

	cfg := &config.Config{
		Listen:   "127.0.0.1:0",
		APIHost:  ts.URL,
		APIToken: "test-token",
		NodeID:   1,
		NodeType: "anytls",
		Log:      config.LogConfig{Level: "debug"},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// First call: should get users
	users, err := srv.apiClient.FetchUsers()
	if err != nil {
		t.Fatalf("first FetchUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}

	// Second call: should get 304 (nil users)
	users, err = srv.apiClient.FetchUsers()
	if err != nil {
		t.Fatalf("second FetchUsers failed: %v", err)
	}
	if users != nil {
		t.Error("expected nil users on 304, got non-nil")
	}

	if callCount.Load() != 2 {
		t.Errorf("expected 2 API calls, got %d", callCount.Load())
	}
}

// TestIntegration_SyncLoopPullPush 测试 syncLoop 的 pull 和 push 周期
func TestIntegration_SyncLoopPullPush(t *testing.T) {
	speedLimit := 10
	mock := newMockXboard([]api.User{
		{ID: 1, UUID: "sync-test-uuid", SpeedLimit: &speedLimit},
	})
	ts := httptest.NewServer(mock)
	defer ts.Close()

	cfg := &config.Config{
		Listen:   "127.0.0.1:0",
		APIHost:  ts.URL,
		APIToken: "test-token",
		NodeID:   7,
		NodeType: "anytls",
		Log:      config.LogConfig{Level: "debug"},
	}

	srv, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer failed: %v", err)
	}

	// Set nodeConfig with very short intervals for testing
	srv.nodeConfig = &api.NodeConfig{
		ServerPort: 9443,
		BaseConfig: struct {
			PushInterval int `json:"push_interval"`
			PullInterval int `json:"pull_interval"`
		}{PushInterval: 1, PullInterval: 1},
	}

	// Add some traffic before starting sync
	srv.trafficCounter.Add(1, 100, 200)
	srv.aliveTracker.Track(1, "192.168.1.1")

	// Run syncLoop briefly
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		srv.syncLoop(ctx)
		close(done)
	}()

	// Wait for at least one pull+push cycle
	time.Sleep(2500 * time.Millisecond)
	cancel()
	<-done

	configCalls, userCalls, pushCalls, aliveCalls, statusCalls := mock.getStats()
	// Should have at least 1 pull (config + user) and 1 push (traffic + alive + status)
	if configCalls < 1 {
		t.Errorf("expected config calls >= 1, got %d", configCalls)
	}
	if userCalls < 1 {
		t.Errorf("expected user calls >= 1, got %d", userCalls)
	}
	if pushCalls < 1 {
		t.Errorf("expected push calls >= 1, got %d", pushCalls)
	}
	if aliveCalls < 1 {
		t.Errorf("expected alive calls >= 1, got %d", aliveCalls)
	}
	if statusCalls < 1 {
		t.Errorf("expected status calls >= 1, got %d", statusCalls)
	}

	// Verify user was synced
	user := srv.userManager.GetUser(1)
	if user == nil {
		t.Error("expected user 1 to be synced")
	} else if user.SpeedLimit != 10 {
		t.Errorf("expected speed_limit=10, got %d", user.SpeedLimit)
	}

	// Verify traffic was pushed (snapshot should have cleared it)
	lastTraffic := mock.getLastTraffic()
	if lastTraffic != nil {
		if t1, ok := lastTraffic["1"]; ok {
			if t1[0] != 100 || t1[1] != 200 {
				t.Errorf("unexpected traffic: %v", t1)
			}
		}
	}

	fmt.Println("syncLoop integration test passed")
}
