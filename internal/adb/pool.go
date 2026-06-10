package adb

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ... (no change)

// DevicePool 设备连接池（支持多模拟器并发管理）
type DevicePool struct {
	mu       sync.RWMutex
	sessions map[string]*DeviceSession
	ttl      time.Duration
	adb      *ADBHelper
}

// NewDevicePool 创建设备连接池
func NewDevicePool(adb *ADBHelper, ttl time.Duration) *DevicePool {
	return &DevicePool{
		sessions: make(map[string]*DeviceSession),
		ttl:      ttl,
		adb:      adb,
	}
}

// GetOrConnect 获取或创建设备会话
func (p *DevicePool) GetOrConnect(host string, port int) (*DeviceSession, error) {
	address := formatAddress(host, port)

	p.mu.RLock()
	session, exists := p.sessions[address]
	p.mu.RUnlock()

	if exists && !session.IsExpired() {
		return session, nil
	}

	// 需要连接
	p.mu.Lock()
	defer p.mu.Unlock()

	// 双重检查
	session, exists = p.sessions[address]
	if exists && !session.IsExpired() {
		return session, nil
	}

	ok, msg := p.adb.Connect(host, port)
	if !ok {
		slog.Warn("设备连接失败", "address", address, "msg", msg)
		return nil, &DeviceError{Message: msg}
	}

	session = &DeviceSession{
		Address:   address,
		CreatedAt: time.Now(),
		TTL:       p.ttl,
	}
	p.sessions[address] = session
	slog.Info("设备已加入连接池", "address", address)
	return session, nil
}

// Disconnect 断开并移除设备
func (p *DevicePool) Disconnect(host string, port int) {
	address := formatAddress(host, port)
	p.mu.Lock()
	delete(p.sessions, address)
	p.mu.Unlock()
	p.adb.Disconnect(host, port)
}

// CleanupExpired 清理过期会话
func (p *DevicePool) CleanupExpired() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for addr, session := range p.sessions {
		if session.IsExpired() {
			delete(p.sessions, addr)
			slog.Debug("设备会话已过期，从池中移除", "address", addr)
		}
	}
}

// ListDevices 列出所有活跃设备
func (p *DevicePool) ListDevices() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var addrs []string
	for addr := range p.sessions {
		addrs = append(addrs, addr)
	}
	return addrs
}

// ADB 返回底层的 ADBHelper 实例
func (p *DevicePool) ADB() *ADBHelper {
	return p.adb
}

func formatAddress(host string, port int) string {
	return fmt.Sprintf("%s:%d", host, port)
}

// DeviceError 设备错误
type DeviceError struct {
	Message string
}

func (e *DeviceError) Error() string {
	return e.Message
}
