package automator

import (
	"sync"
	"time"
)

// mockDeviceOperator 模拟 DeviceOperator 接口，用于单元测试
type mockDeviceOperator struct {
	mu sync.Mutex

	// 可配置的返回值
	dumpHierarchy   func() (string, error)
	click           func(x, y int) error
	clickText       func(text string) bool
	textExists      func(text string) bool
	openApp         func(packageName string) bool
	closeApp        func(packageName string) bool
	screenshot      func() ([]byte, error)
	windowSize      func() (width, height int, err error)
	sendKeyEvent    func(keyCode int) error

	// 调用记录（用于断言）
	calls    []string
	callArgs []map[string]interface{}
}

func newMockDevice() *mockDeviceOperator {
	return &mockDeviceOperator{
		dumpHierarchy: func() (string, error) { return "", nil },
		click:         func(x, y int) error { return nil },
		clickText:     func(text string) bool { return false },
		textExists:    func(text string) bool { return false },
		openApp:       func(packageName string) bool { return true },
		closeApp:      func(packageName string) bool { return true },
		screenshot:    func() ([]byte, error) { return []byte{}, nil },
		windowSize:    func() (width, height int, err error) { return 1080, 1920, nil },
		sendKeyEvent:  func(keyCode int) error { return nil },
	}
}

// recordCall 记录每一次接口调用
func (m *mockDeviceOperator) recordCall(name string, args map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, name)
	m.callArgs = append(m.callArgs, args)
}

// CallCount 返回指定方法的调用次数
func (m *mockDeviceOperator) CallCount(name string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.calls {
		if c == name {
			count++
		}
	}
	return count
}

// TotalCalls 返回总调用次数
func (m *mockDeviceOperator) TotalCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// Reset 重置调用记录
func (m *mockDeviceOperator) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.callArgs = nil
}

// ── DeviceOperator 接口实现 ──

func (m *mockDeviceOperator) DumpHierarchy() (string, error) {
	m.recordCall("DumpHierarchy", nil)
	return m.dumpHierarchy()
}

func (m *mockDeviceOperator) Click(x, y int) error {
	m.recordCall("Click", map[string]interface{}{"x": x, "y": y})
	return m.click(x, y)
}

func (m *mockDeviceOperator) ClickText(text string) bool {
	m.recordCall("ClickText", map[string]interface{}{"text": text})
	return m.clickText(text)
}

func (m *mockDeviceOperator) TextExists(text string) bool {
	m.recordCall("TextExists", map[string]interface{}{"text": text})
	return m.textExists(text)
}

func (m *mockDeviceOperator) OpenApp(packageName string) bool {
	m.recordCall("OpenApp", map[string]interface{}{"package": packageName})
	return m.openApp(packageName)
}

func (m *mockDeviceOperator) CloseApp(packageName string) bool {
	m.recordCall("CloseApp", map[string]interface{}{"package": packageName})
	return m.closeApp(packageName)
}

func (m *mockDeviceOperator) Screenshot() ([]byte, error) {
	m.recordCall("Screenshot", nil)
	return m.screenshot()
}

func (m *mockDeviceOperator) WindowSize() (width, height int, err error) {
	m.recordCall("WindowSize", nil)
	return m.windowSize()
}

func (m *mockDeviceOperator) SendKeyEvent(keyCode int) error {
	m.recordCall("SendKeyEvent", map[string]interface{}{"keyCode": keyCode})
	return m.sendKeyEvent(keyCode)
}

func (m *mockDeviceOperator) WaitForUIReady(timeout time.Duration, minNodes int) bool {
	m.recordCall("WaitForUIReady", map[string]interface{}{"timeout": timeout, "minNodes": minNodes})
	return true
}

func (m *mockDeviceOperator) IsLoggedIn() bool {
	m.recordCall("IsLoggedIn", nil)
	return true
}

func (m *mockDeviceOperator) IsOnLoginPage() bool {
	m.recordCall("IsOnLoginPage", nil)
	return false
}
