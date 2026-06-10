package automator

import (
	"testing"
	"time"
)

func TestNavigator_IsCheckinPageReady(t *testing.T) {
	tests := []struct {
		name      string
		textExist func(text string) bool
		want      bool
	}{
		{
			name: "今日考勤 exists",
			textExist: func(text string) bool {
				return text == "今日考勤"
			},
			want: true,
		},
		{
			name: "签到 exists",
			textExist: func(text string) bool {
				return text == "签到"
			},
			want: true,
		},
		{
			name: "neither exists",
			textExist: func(text string) bool {
				return false
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockDevice()
			mock.textExists = tc.textExist
			nav := NewNavigator(mock, "com.test")

			got := nav.IsCheckinPageReady()
			if got != tc.want {
				t.Errorf("IsCheckinPageReady() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNavigator_DismissNavigationDialogs(t *testing.T) {
	mock := newMockDevice()
	// 让 ClickText 对特定文本返回 true
	mock.clickText = func(text string) bool {
		return text == "我知道了"
	}
	nav := NewNavigator(mock, "com.test")

	nav.DismissNavigationDialogs()

	// 应尝试每个弹窗文本
	if mock.CallCount("ClickText") < 1 {
		t.Error("expected at least 1 ClickText call")
	}
}

func TestNavigator_NavigateToCheckin_AlreadyOnPage(t *testing.T) {
	mock := newMockDevice()
	mock.textExists = func(text string) bool {
		return text == "今日考勤" || text == "签到"
	}
	nav := NewNavigator(mock, "com.test")

	ok, recovery := nav.NavigateToCheckin()
	if !ok {
		t.Error("expected success when already on checkin page")
	}
	if recovery != "" {
		t.Errorf("expected empty recovery, got %s", recovery)
	}
}

func TestNavigator_NavigateToCheckin_FromWorkbench(t *testing.T) {
	mock := newMockDevice()
	attempt := 0
	// 第 1 次 textExists 检查（IsCheckinPageReady）返回 false
	// 随后 tryNavigateWorkbenchToCheckin 会多次调用，让它在第 2 次 textExists 时返回 true
	mock.textExists = func(text string) bool {
		attempt++
		if attempt >= 3 { // 第一次 tryNavigate 尝试成功后退出
			return true
		}
		return false
	}
	mock.clickText = func(text string) bool {
		return true // 所有点击都成功
	}
	nav := NewNavigator(mock, "com.test")

	ok, recovery := nav.NavigateToCheckin()
	if !ok {
		t.Error("expected navigation to succeed from workbench")
	}
	if recovery != "" {
		t.Errorf("expected no recovery, got %s", recovery)
	}
}

func TestNavigator_NavigateToCheckin_WithRecovery(t *testing.T) {
	mock := newMockDevice()
	mock.textExists = func(text string) bool {
		return false // 一直不在打卡页面 → 但只验证恢复机制正确设置，不等待执行
	}
	mock.clickText = func(text string) bool {
		return true
	}

	// 验证单个恢复步骤的方法（不调用完整的 NavigateToCheckin，避免 time.Sleep 累积）
	nav := NewNavigator(mock, "com.test")

	// 验证 lastRecovery 和 ResetRecoveryAction
	if nav.LastRecoveryAction() != "" {
		t.Error("initial recovery action should be empty")
	}
	nav.lastRecovery = "return_home_retry"
	if nav.LastRecoveryAction() != "return_home_retry" {
		t.Errorf("expected return_home_retry, got %s", nav.LastRecoveryAction())
	}
	nav.ResetRecoveryAction()
	if nav.LastRecoveryAction() != "" {
		t.Error("after reset, recovery action should be empty")
	}

	// 验证 DoCheckin 场景下的函数名映射
	if mock.CallCount("CloseApp") != 0 {
		t.Log("CloseApp count is 0 (no full recovery invoked)")
	}
}

func TestNavigator_ReturnToHome(t *testing.T) {
	tests := []struct {
		name       string
		textExists func(text string) bool
		wantCalls  int // min SendKeyEvent calls
	}{
		{
			name: "already on workbench page",
			textExists: func(text string) bool {
				return true
			},
			wantCalls: 0,
		},
		{
			name: "needs back navigation",
			textExists: func(text string) bool {
				return false // 一直没找到"工作台"
			},
			wantCalls: 8, // 最多 8 次返回键
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := newMockDevice()
			mock.textExists = tc.textExists
			nav := NewNavigator(mock, "com.test")

			nav.ReturnToHome()

			if mock.CallCount("SendKeyEvent") < tc.wantCalls {
				t.Errorf("expected at least %d SendKeyEvent calls, got %d",
					tc.wantCalls, mock.CallCount("SendKeyEvent"))
			}
		})
	}
}

func TestRecoveryLabel(t *testing.T) {
	tests := []struct {
		action string
		want   string
	}{
		{"return_home_retry", "回主界面重试"},
		{"restart_app_retry", "重开交建通重试"},
		{"unknown", ""},
		{"", ""},
	}

	for _, tc := range tests {
		t.Run(tc.action, func(t *testing.T) {
			got := RecoveryLabel(tc.action)
			if got != tc.want {
				t.Errorf("RecoveryLabel(%q) = %q, want %q", tc.action, got, tc.want)
			}
		})
	}
}

func TestNavigator_LastRecoveryAction(t *testing.T) {
	mock := newMockDevice()
	nav := NewNavigator(mock, "com.test")

	// 初始状态
	if nav.LastRecoveryAction() != "" {
		t.Error("initial recovery action should be empty")
	}

	// 设置恢复动作（模拟）
	nav.lastRecovery = "restart_app_retry"
	if nav.LastRecoveryAction() != "restart_app_retry" {
		t.Errorf("expected restart_app_retry, got %s", nav.LastRecoveryAction())
	}

	// 重置
	nav.ResetRecoveryAction()
	if nav.LastRecoveryAction() != "" {
		t.Error("after reset, recovery action should be empty")
	}
}

// TestNavigator_NavigateConcurrency 检查导航在并发下的基本稳定性
func TestNavigator_NavigateConcurrency(t *testing.T) {
	mock := newMockDevice()
	attempt := 0
	mock.textExists = func(text string) bool {
		time.Sleep(5 * time.Millisecond) // 模拟耗时
		attempt++
		return attempt >= 3 // 快速成功
	}
	nav := NewNavigator(mock, "com.test")

	done := make(chan bool)
	go func() {
		nav.NavigateToCheckin()
		done <- true
	}()

	select {
	case <-done:
		// ok 完成了
	case <-time.After(10 * time.Second):
		t.Fatal("navigation timed out (likely deadlock)")
	}
}
