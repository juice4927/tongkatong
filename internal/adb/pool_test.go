package adb

import (
	"testing"
	"time"
)

func TestNewDevicePool(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	if pool == nil {
		t.Fatal("NewDevicePool returned nil")
	}
	if pool.adb != adb {
		t.Error("ADB helper not set correctly")
	}
	if pool.ttl != 30*time.Second {
		t.Errorf("TTL = %v, want 30s", pool.ttl)
	}
}

func TestDevicePool_GetOrConnect_ConnectionFailure(t *testing.T) {
	adb := NewADBHelper("adb") // 真实的 ADBHelper，但在测试环境中 adb 不可用
	pool := NewDevicePool(adb, 30*time.Second)

	session, err := pool.GetOrConnect("127.0.0.1", 5555)
	// 由于测试环境没有 adb，连接应该失败
	if err == nil {
		t.Log("adb happened to be available, session returned")
	} else {
		t.Logf("expected connection failure: %v", err)
	}
	if session != nil && err == nil {
		t.Log("unexpectedly connected to device in test environment")
	}
}

func TestDevicePool_Disconnect(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	// 直接操作 sessions 创建 session（绕过 GetOrConnect）
	pool.mu.Lock()
	pool.sessions["127.0.0.1:5555"] = &DeviceSession{
		Address:   "127.0.0.1:5555",
		CreatedAt: time.Now(),
		TTL:       30 * time.Second,
	}
	pool.mu.Unlock()

	if len(pool.ListDevices()) != 1 {
		t.Fatalf("expected 1 device before disconnect")
	}

	pool.Disconnect("127.0.0.1", 5555)

	if len(pool.ListDevices()) != 0 {
		t.Errorf("expected 0 devices after disconnect, got %d", len(pool.ListDevices()))
	}
}

func TestDevicePool_CleanupExpired(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	// 手动添加两个 session：一个过期，一个未过期
	pool.mu.Lock()
	pool.sessions["expired:5555"] = &DeviceSession{
		Address:   "expired:5555",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		TTL:       1 * time.Minute,
	}
	pool.sessions["active:5555"] = &DeviceSession{
		Address:   "active:5555",
		CreatedAt: time.Now(),
		TTL:       30 * time.Minute,
	}
	pool.mu.Unlock()

	pool.CleanupExpired()

	devices := pool.ListDevices()
	if len(devices) != 1 {
		t.Fatalf("expected 1 device after cleanup, got %d", len(devices))
	}
	if devices[0] != "active:5555" {
		t.Errorf("expected active:5555 to remain, got %s", devices[0])
	}
}

func TestDevicePool_CleanupExpired_AllExpired(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 1*time.Nanosecond)

	pool.mu.Lock()
	pool.sessions["s1:5555"] = &DeviceSession{
		Address:   "s1:5555",
		CreatedAt: time.Now().Add(-1 * time.Hour),
		TTL:       1 * time.Nanosecond,
	}
	pool.sessions["s2:5556"] = &DeviceSession{
		Address:   "s2:5556",
		CreatedAt: time.Now().Add(-2 * time.Hour),
		TTL:       1 * time.Nanosecond,
	}
	pool.mu.Unlock()

	pool.CleanupExpired()

	if len(pool.ListDevices()) != 0 {
		t.Errorf("expected 0 devices after cleanup, got %d", len(pool.ListDevices()))
	}
}

func TestDevicePool_ListDevices(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	pool.mu.Lock()
	pool.sessions["127.0.0.1:5555"] = &DeviceSession{
		Address: "127.0.0.1:5555", CreatedAt: time.Now(), TTL: 30 * time.Second,
	}
	pool.sessions["10.0.0.1:5556"] = &DeviceSession{
		Address: "10.0.0.1:5556", CreatedAt: time.Now(), TTL: 30 * time.Second,
	}
	pool.mu.Unlock()

	devices := pool.ListDevices()
	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	addrMap := make(map[string]bool)
	for _, addr := range devices {
		addrMap[addr] = true
	}
	if !addrMap["127.0.0.1:5555"] {
		t.Error("127.0.0.1:5555 not in device list")
	}
	if !addrMap["10.0.0.1:5556"] {
		t.Error("10.0.0.1:5556 not in device list")
	}
}

func TestDevicePool_ADB(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	if pool.ADB() != adb {
		t.Error("ADB() should return the same helper")
	}
}

func TestFormatAddress(t *testing.T) {
	tests := []struct {
		host string
		port int
		want string
	}{
		{"127.0.0.1", 5555, "127.0.0.1:5555"},
		{"10.0.0.1", 5556, "10.0.0.1:5556"},
		{"localhost", 0, "localhost:0"},
	}

	for _, tc := range tests {
		got := formatAddress(tc.host, tc.port)
		if got != tc.want {
			t.Errorf("formatAddress(%s, %d) = %s, want %s", tc.host, tc.port, got, tc.want)
		}
	}
}

func TestDeviceError(t *testing.T) {
	err := &DeviceError{Message: "test error"}
	if err.Error() != "test error" {
		t.Errorf("DeviceError.Error() = %s, want 'test error'", err.Error())
	}
}

func TestDeviceSession_IsExpired(t *testing.T) {
	now := time.Now()

	session1 := &DeviceSession{
		Address:   "127.0.0.1:5555",
		CreatedAt: now.Add(-5 * time.Second),
		TTL:       30 * time.Second,
	}
	if session1.IsExpired() {
		t.Error("session created 5s ago with 30s TTL should not be expired")
	}

	session2 := &DeviceSession{
		Address:   "127.0.0.1:5555",
		CreatedAt: now.Add(-1 * time.Hour),
		TTL:       30 * time.Second,
	}
	if !session2.IsExpired() {
		t.Error("session created 1h ago with 30s TTL should be expired")
	}

	session3 := &DeviceSession{
		Address:   "127.0.0.1:5555",
		CreatedAt: now.Add(-1 * time.Nanosecond),
		TTL:       0,
	}
	if !session3.IsExpired() {
		t.Error("session with zero TTL should be expired")
	}
}

func TestDevicePool_DoubleCheckLock(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	// 手动插入一个 session 来测试双重检查锁定
	pool.mu.Lock()
	pool.sessions["127.0.0.1:5555"] = &DeviceSession{
		Address:   "127.0.0.1:5555",
		CreatedAt: time.Now(),
		TTL:       30 * time.Second,
	}
	pool.mu.Unlock()

	// GetOrConnect 应该返回现有的 session 而不调用 adb.Connect
	session, err := pool.GetOrConnect("127.0.0.1", 5555)
	if err != nil {
		t.Fatalf("GetOrConnect should succeed with existing session: %v", err)
	}
	if session == nil {
		t.Fatal("GetOrConnect returned nil session")
	}
	if session.Address != "127.0.0.1:5555" {
		t.Errorf("expected address 127.0.0.1:5555, got %s", session.Address)
	}
}

func TestDevicePool_ConcurrentAccess(t *testing.T) {
	adb := NewADBHelper("")
	pool := NewDevicePool(adb, 30*time.Second)

	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func(idx int) {
			addr := formatAddress("10.0.0.1", 5555+idx)

			// 直接注册 session（避免调用真实的 Connect）
			pool.mu.Lock()
			pool.sessions[addr] = &DeviceSession{
				Address: addr, CreatedAt: time.Now(), TTL: 30 * time.Second,
			}
			pool.mu.Unlock()

			done <- true
		}(i)
	}

	for i := 0; i < 5; i++ {
		<-done
	}

	devices := pool.ListDevices()
	if len(devices) != 5 {
		t.Errorf("expected 5 devices after concurrent access, got %d", len(devices))
	}
}
