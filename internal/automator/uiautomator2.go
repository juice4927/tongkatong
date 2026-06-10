package automator

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/models"
)

// ── 自定义错误 ──────────────────────────────────────────────────────

type DeviceConnectionError struct {
	Message     string
	FailureCode string
}

func (e *DeviceConnectionError) Error() string { return e.Message }

type AppNotFoundError struct{ Message string }

func (e *AppNotFoundError) Error() string { return e.Message }

type LoginTimeoutError struct{ Message string }

func (e *LoginTimeoutError) Error() string { return e.Message }

type GpsLocationError struct{ Message string }

func (e *GpsLocationError) Error() string { return e.Message }

type AlreadyCheckedInError struct {
	Message         string
	InCorrectSlot   bool
	CheckinTime     string
}

func (e *AlreadyCheckedInError) Error() string { return e.Message }

// ── UIAutomator2Impl ──────────────────────────────────────────────

// UIAutomator2Impl uiautomator2 模式实现
type UIAutomator2Impl struct {
	host        string
	port        int
	adbPath     string
	packageName string
	pool        *adb.DevicePool
	adbHelper   *adb.ADBHelper

	mu        sync.Mutex
	connected bool
	deviceAddr string

	navigator *Navigator
	verifier  *CheckinVerifier
}

// NewUIAutomator2Impl 创建自动化执行器
func NewUIAutomator2Impl(host string, port int, adbPath string, packageName string, pool *adb.DevicePool) *UIAutomator2Impl {
	if packageName == "" {
		packageName = "com.tencent.weworklocal"
	}

	impl := &UIAutomator2Impl{
		host:        host,
		port:        port,
		adbPath:     adbPath,
		packageName: packageName,
		pool:        pool,
		adbHelper:   pool.ADB(),
		deviceAddr:  fmt.Sprintf("%s:%d", host, port),
	}

	impl.navigator = NewNavigator(impl, packageName)
	impl.verifier = NewCheckinVerifier(impl, packageName)

	return impl
}

// Connect 连接设备
func (u *UIAutomator2Impl) Connect() (bool, string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	slog.Info("正在连接设备", "address", u.deviceAddr)
	ok, msg := u.adbHelper.Connect(u.host, u.port)
	if ok {
		u.connected = true
		slog.Info("设备连接成功")
	} else {
		slog.Warn("设备连接失败", "msg", msg)
	}
	return ok, msg
}

// Disconnect 断开连接
func (u *UIAutomator2Impl) Disconnect() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.adbHelper.Disconnect(u.host, u.port)
	u.connected = false
}

// IsConnected 检查是否已连接
func (u *UIAutomator2Impl) IsConnected() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.connected
}

// ── DeviceOperator 接口实现 ───────────────────────────────────────

// DumpHierarchy 获取 UI hierarchy XML
func (u *UIAutomator2Impl) DumpHierarchy() (string, error) {
	return u.adbHelper.DumpHierarchy("")
}

// Click 通过坐标点击
func (u *UIAutomator2Impl) Click(x, y int) error {
	ok, msg := u.adbHelper.Shell("", fmt.Sprintf("input tap %d %d", x, y), 5*time.Second)
	if !ok {
		return fmt.Errorf("点击失败: %s", msg)
	}
	return nil
}

// ClickText 按文本查找并点击
func (u *UIAutomator2Impl) ClickText(text string) bool {
	// 通过 uiautomator 命令查找并点击
	// 格式: uiautomator 不支持直接文本点击，改用 adb shell 命令
	// 使用 input tap + XML bounds 的方式
	xml, err := u.DumpHierarchy()
	if err != nil {
		return false
	}

	nodes := ParseHierarchyXML(xml)
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == text && isClickable(n.Clickable) {
			err := u.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY())
			if err == nil {
				return true
			}
		}
	}

	// 放宽条件：不要求 clickable
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == text {
			err := u.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY())
			if err == nil {
				time.Sleep(500 * time.Millisecond)
				return true
			}
		}
	}

	return false
}

// TextExists 检查文本是否存在
func (u *UIAutomator2Impl) TextExists(text string) bool {
	xml, err := u.DumpHierarchy()
	if err != nil {
		return false
	}
	nodes := ParseHierarchyXML(xml)
	for _, n := range nodes {
		if strings.Contains(n.Text, text) {
			return true
		}
	}
	return false
}

// OpenApp 打开应用
//
//	先通过 monkey 方式启动；失败则查询 launcher activity 后使用 am start
func (u *UIAutomator2Impl) OpenApp(packageName string) bool {
	// 方法1：monkey 启动（通用，适用于任何包名）
	ok, _ := u.adbHelper.Shell("", fmt.Sprintf("monkey -p %s -c android.intent.category.LAUNCHER 1", packageName), 10*time.Second)
	if ok {
		return true
	}

	// 方法2：查询 launcher activity 并启动（不再硬编码 WwMainActivity）
	ok2, activity := u.adbHelper.Shell("", fmt.Sprintf("cmd package resolve-activity --brief %s 2>/dev/null || pm resolve-activity --brief %s 2>/dev/null", packageName, packageName), 10*time.Second)
	if ok2 && activity != "" {
		lines := strings.Split(strings.TrimSpace(activity), "\n")
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if lastLine != "" && strings.Contains(lastLine, "/") {
			ok3, _ := u.adbHelper.Shell("", fmt.Sprintf("am start -n %s", lastLine), 10*time.Second)
			return ok3
		}
	}

	// 方法3：通用兜底 — 直接启动主 activity
	ok4, _ := u.adbHelper.Shell("", fmt.Sprintf("am start -n %s/.MainActivity", packageName), 10*time.Second)
	if ok4 {
		return true
	}
	ok5, _ := u.adbHelper.Shell("", fmt.Sprintf("am start -n %s/.SplashActivity", packageName), 10*time.Second)
	return ok5
}

// CloseApp 关闭应用
func (u *UIAutomator2Impl) CloseApp(packageName string) bool {
	ok, _ := u.adbHelper.Shell("", fmt.Sprintf("am force-stop %s", packageName), 5*time.Second)
	return ok
}

// Screenshot 截屏
func (u *UIAutomator2Impl) Screenshot() ([]byte, error) {
	return u.adbHelper.Screencap("")
}

// WindowSize 获取屏幕尺寸
func (u *UIAutomator2Impl) WindowSize() (int, int, error) {
	ok, out := u.adbHelper.Shell("", "wm size", 5*time.Second)
	if !ok {
		return 1080, 1920, fmt.Errorf("获取屏幕尺寸失败: %s", out)
	}
	// 输出格式: Physical size: 1080x1920
	parts := strings.Split(strings.TrimSpace(out), " ")
	if len(parts) >= 1 {
		last := parts[len(parts)-1]
		dim := strings.Split(last, "x")
		if len(dim) == 2 {
			w, _ := parseInt(dim[0])
			h, _ := parseInt(dim[1])
			if w > 0 && h > 0 {
				return w, h, nil
			}
		}
	}
	return 1080, 1920, nil
}

// SendKeyEvent 发送按键事件
func (u *UIAutomator2Impl) SendKeyEvent(keyCode int) error {
	ok, msg := u.adbHelper.Shell("", fmt.Sprintf("input keyevent %d", keyCode), 5*time.Second)
	if !ok {
		return fmt.Errorf("发送按键事件 %d 失败: %s", keyCode, msg)
	}
	return nil
}

// ── 打卡执行 ───────────────────────────────────────────────────────

// DoCheckin 执行一次完整的打卡流程
func (u *UIAutomator2Impl) DoCheckin(action CheckinAction) (*models.CheckinResult, error) {
	_, isSignin, slotLabel := ResolveActionSlot(action)
	actionText := "签到"
	if !isSignin {
		actionText = "签退"
	}

	slog.Info("开始执行打卡", "action", action, "slot", slotLabel, "text", actionText)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// 1. 检查是否已连接
	if !u.IsConnected() {
		if ok, _ := u.Connect(); !ok {
			return &models.CheckinResult{
				Success:     false,
				Action:      string(action),
				Message:     "设备未连接",
				Timestamp:   timestamp,
				FailureCode: string(models.DeviceNotConnected),
			}, nil
		}
	}

	// 2. 打卡前检查是否已打卡
	if IsAlreadyCheckedIn(action, u) {
		slog.Info("该时段已打卡，跳过")
		return &models.CheckinResult{
			Success:   true,
			Action:    string(action),
			Message:   "该时段已打卡，跳过",
			Timestamp: timestamp,
		}, nil
	}

	// 3. 打开应用
	slog.Info("打开应用", "package", u.packageName)
	if !u.OpenApp(u.packageName) {
		return &models.CheckinResult{
			Success:     false,
			Action:      string(action),
			Message:     "应用启动失败",
			Timestamp:   timestamp,
			FailureCode: string(models.AppNotFound),
		}, nil
	}
	time.Sleep(3 * time.Second)

	// 4. 导航到考勤页面
	slog.Info("导航到考勤页面")
	navOk, recovery := u.navigator.NavigateToCheckin()
	if !navOk {
		return &models.CheckinResult{
			Success:        false,
			Action:         string(action),
			Message:        "导航到考勤页面失败",
			Timestamp:      timestamp,
			FailureCode:    string(models.NavigationFailed),
			RecoveryAction: recovery,
		}, nil
	}

	// 5. 查找并点击打卡按钮
	slog.Info("查找打卡按钮", "text", actionText)
	if !u.findAndClickButton(actionText) {
		return &models.CheckinResult{
			Success:     false,
			Action:      string(action),
			Message:     "未找到打卡按钮",
			Timestamp:   timestamp,
			FailureCode: string(models.ButtonNotFound),
		}, nil
	}
	time.Sleep(2 * time.Second)

	// 6. 处理可能出现的确认弹窗
	_ = u.verifier.HandleConfirmDialog(30)

	// 7. 验证打卡结果
	slog.Info("验证打卡结果")
	success := u.verifier.DefaultVerify(action)

	message := "打卡成功"
	failureCode := ""
	if !success {
		message = "打卡失败"
		failureCode = string(models.CheckinFailed)
		// 截屏保存诊断
		if data, err := u.Screenshot(); err == nil {
			diagPath := filepath.Join("logs", "diagnosis")
			_ = os.MkdirAll(diagPath, 0755)
			ts := time.Now().Format("20060102_150405")
			savePath := filepath.Join(diagPath, fmt.Sprintf("fail_%s_%s.png", action, ts))
			_ = os.WriteFile(savePath, data, 0644)
			slog.Info("失败诊断截图已保存", "path", savePath)
		}
	}

	// 8. 返回主页
	u.navigator.ReturnToHome()

	return &models.CheckinResult{
		Success:        success,
		Action:         string(action),
		Message:        message,
		Timestamp:      timestamp,
		FailureCode:    failureCode,
		RecoveryAction: u.navigator.LastRecoveryAction(),
	}, nil
}

// findAndClickButton 查找并点击按钮
func (u *UIAutomator2Impl) findAndClickButton(text string) bool {
	xml, err := u.DumpHierarchy()
	if err != nil {
		return false
	}

	nodes := ParseHierarchyXML(xml)

	// 首先尝试直接找可点击的文本按钮
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == text && isClickable(n.Clickable) {
			slog.Info("找到可点击按钮", "text", text, "bounds", n.Bounds)
			err := u.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY())
			return err == nil
		}
	}

	// 策略 A：行锚定法 — 找左侧标签，点击右侧对应区域
	w, _, _ := u.WindowSize()
	midX := int(float64(w) * 0.5)
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if n.Text == text && !isClickable(n.Clickable) && n.BoundsParsed.CenterX() < midX {
			// 找到左侧标签，点击右侧对应区域
			labelRect := n.BoundsParsed
			rightNodes := CollectRightNodes(nodes, labelRect, midX)
			for _, rn := range rightNodes {
				if rn.Clickable && rn.Rect != nil {
					slog.Info("行锚定法：点击右侧时间", "time", rn.Time)
					err := u.Click(rn.Rect.CenterX(), rn.Rect.CenterY())
					return err == nil
				}
			}
		}
	}

	// 策略 C：兜底 — 用 contains 匹配文本，点击任何包含目标文字的可点击元素
	slog.Info("兜底方案：尝试文本包含匹配", "text", text)
	for _, n := range nodes {
		if n.BoundsParsed == nil {
			continue
		}
		if strings.Contains(n.Text, text) {
			err := u.Click(n.BoundsParsed.CenterX(), n.BoundsParsed.CenterY())
			if err == nil {
				return true
			}
		}
	}

	return false
}
