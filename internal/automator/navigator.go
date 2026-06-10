package automator

import (
	"log/slog"
	"time"
)

// ── 导航恢复模块 ────────────────────────────────────────────────────

// Navigator 导航与恢复引擎
//
//	从交建通主界面导航到考勤打卡页面
//	导航失败时自动执行多级恢复策略
type Navigator struct {
	device     DeviceOperator
	packageName string
	lastRecovery string
}

// DeviceOperator 设备操作接口（由 UIAutomator2Impl 实现）
type DeviceOperator interface {
	DumpHierarchy() (string, error)
	Click(x, y int) error
	ClickText(text string) bool
	TextExists(text string) bool
	OpenApp(packageName string) bool
	CloseApp(packageName string) bool
	Screenshot() ([]byte, error)
	WindowSize() (width, height int, err error)
	SendKeyEvent(keyCode int) error
	WaitForUIReady(timeout time.Duration, minNodes int) bool
	IsLoggedIn() bool
	IsOnLoginPage() bool
}

// NewNavigator 创建导航器
func NewNavigator(device DeviceOperator, packageName string) *Navigator {
	return &Navigator{
		device:      device,
		packageName: packageName,
	}
}

// LastRecoveryAction 返回最近一次恢复动作
func (n *Navigator) LastRecoveryAction() string {
	return n.lastRecovery
}

// ResetRecoveryAction 重置恢复动作
func (n *Navigator) ResetRecoveryAction() {
	n.lastRecovery = ""
}

// RecoveryLabel 返回恢复动作的中文描述
func RecoveryLabel(action string) string {
	switch action {
	case "return_home_retry":
		return "回主界面重试"
	case "restart_app_retry":
		return "重开交建通重试"
	}
	return ""
}

// DismissNavigationDialogs 关闭导航时常见的提示弹窗
func (n *Navigator) DismissNavigationDialogs() {
	for _, text := range []string{"我知道了", "知道了", "确定", "关闭", "取消"} {
		if n.device.ClickText(text) {
			slog.Info("关闭导航弹窗", "text", text)
			time.Sleep(1 * time.Second)
		}
	}
}

// IsCheckinPageReady 检查是否到达考勤页面（看到"今日考勤"或"签到"）
func (n *Navigator) IsCheckinPageReady() bool {
	// 用 ClickText 的检查逻辑判断元素是否存在（但不点击）
	return n.device.TextExists("今日考勤") || n.device.TextExists("签到")
}

// NavigateToCheckin 导航到考勤页面
//
//	返回: (是否成功, 恢复动作描述)
func (n *Navigator) NavigateToCheckin() (bool, string) {
	n.lastRecovery = ""

	// 先检查是否已经在考勤页面
	if n.IsCheckinPageReady() {
		slog.Info("已在考勤页面")
		return true, ""
	}

	// 尝试从工作台导航
	if n.tryNavigateWorkbenchToCheckin(3) {
		return true, ""
	}

	// 第一级恢复：返回主界面重试
	slog.Warn("导航失败，尝试返回主界面重试")
	n.lastRecovery = "return_home_retry"
	n.device.CloseApp(n.packageName)
	time.Sleep(2 * time.Second)
	n.device.OpenApp(n.packageName)
	time.Sleep(3 * time.Second)
	n.DismissNavigationDialogs()

	if n.tryNavigateWorkbenchToCheckin(3) {
		return true, n.lastRecovery
	}

	// 第二级恢复：重启 APP 重试
	slog.Warn("导航再次失败，尝试重启 APP")
	n.lastRecovery = "restart_app_retry"
	n.device.CloseApp(n.packageName)
	time.Sleep(2 * time.Second)
	n.device.OpenApp(n.packageName)
	time.Sleep(5 * time.Second)
	n.DismissNavigationDialogs()

	if n.tryNavigateWorkbenchToCheckin(3) {
		return true, n.lastRecovery
	}

	slog.Error("导航彻底失败，所有恢复策略均无效")
	return false, n.lastRecovery
}

// tryNavigateWorkbenchToCheckin 尝试从工作台进入考勤页面
func (n *Navigator) tryNavigateWorkbenchToCheckin(maxAttempts int) bool {
	for i := 0; i < maxAttempts; i++ {
		if n.IsCheckinPageReady() {
			return true
		}

		// 模拟点击"工作台"入口 → "考勤打卡"
		// 交建通通常的路径：底部"工作台"tab → 找到"考勤打卡"入口
		if n.device.ClickText("工作台") {
			time.Sleep(2 * time.Second)
		}

		// 尝试点击"考勤打卡"或"考勤"
		if n.device.ClickText("考勤打卡") || n.device.ClickText("考勤") || n.device.ClickText("签到") {
			time.Sleep(2 * time.Second)
		}

		// 关闭可能的弹窗
		n.DismissNavigationDialogs()

		if n.IsCheckinPageReady() {
			return true
		}

		if i < maxAttempts-1 {
			time.Sleep(1 * time.Second)
		}
	}
	return false
}

// ReturnToHome 打卡完成后返回工作台
//
//	使用 ADB keyevent KEYCODE_BACK 逐层返回直到看到"工作台"入口
func (n *Navigator) ReturnToHome() {
	for i := 0; i < 8; i++ {
		if n.device.TextExists("工作台") {
			slog.Debug("已回到工作台页面")
			return
		}
		// 发送系统返回键 (KEYCODE_BACK = 4)
		if err := n.device.SendKeyEvent(4); err != nil {
			slog.Warn("发送返回键失败", "error", err)
		}
		time.Sleep(1 * time.Second)
	}
	slog.Debug("返回工作台操作完成（已达最大重试次数）")
}
