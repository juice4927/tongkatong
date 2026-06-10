package automator

import (
	"testing"
)

func TestDialogHandler_ClickButtonByText_FoundByClickText(t *testing.T) {
	mock := newMockDevice()
	mock.clickText = func(text string) bool {
		return text == "确定"
	}
	dh := NewDialogHandler(mock)

	result := dh.ClickButtonByText([]string{"确定", "知道了", "关闭"})
	if !result {
		t.Error("expected true when ClickText finds the button")
	}
	if mock.CallCount("ClickText") < 1 {
		t.Error("expected at least 1 ClickText call")
	}
}

func TestDialogHandler_ClickButtonByText_NotFoundFallback(t *testing.T) {
	// ClickText 找不到任何按钮 -> 应 fallback 到比例点击
	mock := newMockDevice()
	mock.clickText = func(text string) bool {
		return false // 没有按钮被找到
	}
	// 让 DumpHierarchy 返回空 XML 避免镜像逻辑干扰
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0"></hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	result := dh.ClickButtonByText([]string{"确定", "取消"})
	// 当 ClickText 找不到时，应 fallback 到比例点击（Click）
	if !result {
		t.Error("expected fallback to Click to succeed")
	}
	if mock.CallCount("Click") == 0 {
		t.Error("expected Click to be called as fallback")
	}
}

func TestDialogHandler_ClickButtonByText_WithCancelMirror(t *testing.T) {
	mock := newMockDevice()
	mock.clickText = func(text string) bool {
		return false // 找不到直接按钮
	}
	mock.dumpHierarchy = func() (string, error) {
		// 镜像法：提供"取消"按钮的位置从而定位"确定"
		return `<hierarchy rotation="0">
			<node text="取消" bounds="[100,900][300,1000]" clickable="true"/>
		</hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	// texts 包含 "取消" -> 走镜像法
	result := dh.ClickButtonByText([]string{"取消", "关闭", "返回"})
	if !result {
		t.Error("expected mirror click to succeed")
	}
}

func TestDialogHandler_ClickButtonByText_WithConfirmMirror(t *testing.T) {
	mock := newMockDevice()
	mock.clickText = func(text string) bool {
		return false
	}
	mock.dumpHierarchy = func() (string, error) {
		// 镜像法：提供"确定"的位置从而定位"取消"
		return `<hierarchy rotation="0">
			<node text="确定" bounds="[400,900][600,1000]" clickable="true"/>
		</hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	// 当 texts 包含 "确定" -> 走镜像法找"取消"
	result := dh.ClickButtonByText([]string{"确定", "确认"})
	if !result {
		t.Error("expected mirror-based confirm click to succeed")
	}
}

func TestDialogHandler_ClickByMirror_Found(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="确定" bounds="[400,900][600,1000]" clickable="true"/>
		</hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	// 调用 clickByMirror 直接测试
	result := dh.clickByMirror("确定", 1080)
	if !result {
		t.Error("expected clickByMirror to succeed")
	}
	// 应调用 DumpHierarchy 和 Click
	if mock.CallCount("DumpHierarchy") < 1 {
		t.Error("expected DumpHierarchy call")
	}
	if mock.CallCount("Click") < 1 {
		t.Error("expected Click call")
	}
}

func TestDialogHandler_ClickByMirror_NotFound(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0">
			<node text="其他文本" bounds="[400,900][600,1000]" clickable="true"/>
		</hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	result := dh.clickByMirror("确定", 1080)
	if result {
		t.Error("expected clickByMirror to fail when known text not found")
	}
}

func TestDialogHandler_ClickByMirror_DumpError(t *testing.T) {
	mock := newMockDevice()
	mock.dumpHierarchy = func() (string, error) {
		return "", assertError("dump error")
	}

	dh := NewDialogHandler(mock)

	result := dh.clickByMirror("确定", 1080)
	if result {
		t.Error("expected clickByMirror to fail on dump error")
	}
}

func TestDialogHandler_WindowSizeFallback(t *testing.T) {
	mock := newMockDevice()
	mock.clickText = func(text string) bool {
		return false
	}
	mock.windowSize = func() (width, height int, err error) {
		return 0, 0, assertError("window size error")
	}
	mock.dumpHierarchy = func() (string, error) {
		return `<hierarchy rotation="0"></hierarchy>`, nil
	}

	dh := NewDialogHandler(mock)

	result := dh.ClickButtonByText([]string{"确定"})
	if !result {
		t.Error("expected fallback to succeed even with window size error")
	}
	// 窗口尺寸失败时使用默认 1080x1920
	if mock.CallCount("Click") == 0 {
		t.Error("expected Click to be called with default screen size fallback")
	}
}

// assertError 是一个简单的 error 实现，用于测试
type assertError string

func (e assertError) Error() string { return string(e) }
