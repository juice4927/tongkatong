package automator

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/utils"
)

// packageNamePattern Android 包名/activity名校验（字母、数字、点、下划线、$）
var packageNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9._\$]*$`)

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

	baseDir   string // 工作目录，用于诊断文件输出
	mumuHelper *adb.MuMuHelper // GPS 设置委托

	navigator *Navigator
	verifier  *CheckinVerifier
}

// NewUIAutomator2Impl 创建自动化执行器
func NewUIAutomator2Impl(host string, port int, adbPath string, packageName string, pool *adb.DevicePool) *UIAutomator2Impl {
	if packageName == "" {
		packageName = "com.tencent.weworklocal"
	}

	var adbHelper *adb.ADBHelper
	if pool != nil {
		adbHelper = pool.ADB()
	}
	if adbHelper == nil {
		adbHelper = adb.NewADBHelper(adbPath)
	}

	impl := &UIAutomator2Impl{
		host:        host,
		port:        port,
		adbPath:     adbPath,
		packageName: packageName,
		pool:        pool,
		adbHelper:   adbHelper,
		deviceAddr:  fmt.Sprintf("%s:%d", host, port),
	}

	impl.navigator = NewNavigator(impl, packageName)
	impl.verifier = NewCheckinVerifier(impl, packageName)

	return impl
}

// SetBaseDir 设置工作目录（用于诊断文件输出）
func (u *UIAutomator2Impl) SetBaseDir(dir string) {
	u.baseDir = dir
}

// SetMuMuHelper 设置 MuMu 管理器（用于 GPS 等模拟器操作）
func (u *UIAutomator2Impl) SetMuMuHelper(h *adb.MuMuHelper) {
	u.mumuHelper = h
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
	// 安全校验：包名只能是字母数字点下划线
	if !packageNamePattern.MatchString(packageName) {
		slog.Error("非法包名，拒绝执行", "package", packageName)
		return false
	}
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
			// 校验 activity 名称防止命令注入
			if packageNamePattern.MatchString(strings.ReplaceAll(lastLine, "/", ".")) {
				ok3, _ := u.adbHelper.Shell("", fmt.Sprintf("am start -n %s", lastLine), 10*time.Second)
				return ok3
			}
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
	if !packageNamePattern.MatchString(packageName) {
		slog.Error("非法包名，拒绝执行", "package", packageName)
		return false
	}
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
			w, err := strconv.Atoi(dim[0])
			h, err2 := strconv.Atoi(dim[1])
			if err != nil || err2 != nil {
				return 1080, 1920, nil
			}
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

// ── UI 就绪检测 ──────────────────────────────────────────────────

// WaitForUIReady 等待 UI 层级加载完成（至少 minNodes 个节点）
func (u *UIAutomator2Impl) WaitForUIReady(timeout time.Duration, minNodes int) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		xml, err := u.DumpHierarchy()
		if err != nil {
			continue
		}
		nodes := ParseHierarchyXML(xml)
		if len(nodes) >= minNodes {
			return true
		}
	}
	return false
}

// IsLoggedIn 检查是否已在交建通主界面（非登录页）
func (u *UIAutomator2Impl) IsLoggedIn() bool {
	xml, err := u.DumpHierarchy()
	if err != nil {
		return false
	}
	// 检测工作台/考勤入口是否存在
	lower := strings.ToLower(xml)
	return strings.Contains(lower, "工作台") || strings.Contains(lower, "考勤")
}

// IsOnLoginPage 检测是否在登录页面
func (u *UIAutomator2Impl) IsOnLoginPage() bool {
	xml, err := u.DumpHierarchy()
	if err != nil {
		return false
	}
	lower := strings.ToLower(xml)
	return strings.Contains(lower, "登录") || strings.Contains(lower, "login") ||
		strings.Contains(lower, "手机号") || strings.Contains(lower, "验证码")
}

// HandleLoginIfNeeded 如果需要登录则等待用户手动登录
func (u *UIAutomator2Impl) HandleLoginIfNeeded(waitSeconds int) error {
	if !u.IsOnLoginPage() && u.IsLoggedIn() {
		return nil // 已登录，无需处理
	}

	if !u.IsOnLoginPage() && !u.IsLoggedIn() {
		// 既不在登录页也不在主界面 → 可能应用未启动
		return fmt.Errorf("应用状态异常：不在登录页也不在主界面")
	}

	slog.Info("检测到登录页面，等待手动登录", "max_wait", waitSeconds)
	deadline := time.Now().Add(time.Duration(waitSeconds) * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(3 * time.Second)
		if !u.IsOnLoginPage() && u.IsLoggedIn() {
			slog.Info("用户已登录")
			return nil
		}
	}
	return fmt.Errorf("登录超时（%d秒），请先手动登录交建通", waitSeconds)
}

// SetGPS 通过 MuMuManager 设置 GPS 虚拟定位
func (u *UIAutomator2Impl) SetGPS(latitude, longitude float64) error {
	if latitude == 0 && longitude == 0 {
		return nil
	}

	if u.mumuHelper != nil {
		ok, msg := u.mumuHelper.SetGPS(latitude, longitude)
		if !ok {
			return fmt.Errorf("GPS 设置失败: %s", msg)
		}
		return nil
	}

	// 降级：硬编码路径搜索（当 MuMuHelper 不可用时）
	managerPaths := []string{
		`D:\MuMuPlayer\nx_device\12.0\nx_main\MuMuManager.exe`,
		`C:\MuMuPlayer\nx_device\12.0\nx_main\MuMuManager.exe`,
	}

	var managerPath string
	for _, p := range managerPaths {
		if _, err := os.Stat(p); err == nil {
			managerPath = p
			break
		}
	}
	if managerPath == "" {
		return fmt.Errorf("未找到 MuMuManager.exe")
	}

	cmd := exec.Command(managerPath, "control", "-v", "0", "tool", "location",
		"-lat", fmt.Sprintf("%f", latitude),
		"-lon", fmt.Sprintf("%f", longitude),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("GPS 设置失败: %v (output: %s)", err, string(output))
	}
	slog.Info("GPS 已设置", "lat", latitude, "lon", longitude)
    return nil
}

// ── 打卡执行 ───────────────────────────────────────────────────────

// DoCheckin 执行一次完整的打卡流程（含重试+多策略+分类错误）
func (u *UIAutomator2Impl) DoCheckin(action CheckinAction) (*models.CheckinResult, error) {
	return u.doCheckinWithRetry(action, 2)
}

func (u *UIAutomator2Impl) doCheckinWithRetry(action CheckinAction, maxRetries int) (*models.CheckinResult, error) {
	_, isSignin, slotLabel := ResolveActionSlot(action)
	actionText := "签到"
	if !isSignin {
		actionText = "签退"
	}

	var lastResult *models.CheckinResult
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			slog.Info("重试打卡", "attempt", attempt, "max", maxRetries)
			time.Sleep(30 * time.Second)
		}

		result, err := u.doCheckinOnce(action, actionText, slotLabel)
		if err == nil && result != nil && result.Success {
			return result, nil
		}

		lastResult = result
		lastErr = err

		// 不可重试的错误码 → 直接返回
		if result != nil {
			if !ShouldRetryCode(result.FailureCode) {
				slog.Info("错误不可重试，直接返回", "code", result.FailureCode)
				return result, nil
			}
		}

		// 连接错误 → 尝试重连
		if err != nil {
			if _, ok := err.(*DeviceConnectionError); ok {
				slog.Info("设备连接错误，尝试重连")
				u.Connect()
			}
		}
	}

	if lastResult != nil {
		return lastResult, lastErr
	}
	return &models.CheckinResult{
		Success:     false,
		Action:      string(action),
		Message:     "打卡失败：已达最大重试次数",
		Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
		FailureCode: string(models.CheckinFailed),
	}, lastErr
}

func (u *UIAutomator2Impl) doCheckinOnce(action CheckinAction, actionText, slotLabel string) (*models.CheckinResult, error) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// 1. 确保连接
	if !u.IsConnected() {
		if ok, _ := u.Connect(); !ok {
			return &models.CheckinResult{
				Success:     false,
				Action:      string(action),
				Message:     "设备未连接",
				Timestamp:   timestamp,
				FailureCode: string(models.DeviceNotConnected),
			}, &DeviceConnectionError{Message: "设备未连接"}
		}
	}

	// 2. 等待 UI 就绪
	if !u.WaitForUIReady(5*time.Second, 10) {
		slog.Warn("UI 未就绪（节点数不足），继续尝试")
	}

	// 3. 已打卡检测
	if IsAlreadyCheckedIn(action, u) {
		slog.Info("该时段已打卡，跳过")
		return &models.CheckinResult{
			Success:   true,
			Action:    string(action),
			Message:   "该时段已打卡，跳过",
			Timestamp: timestamp,
		}, nil
	}

	// 4. 打开应用
	if !u.OpenApp(u.packageName) {
		return &models.CheckinResult{
			Success:     false,
			Action:      string(action),
			Message:     "应用启动失败",
			Timestamp:   timestamp,
			FailureCode: string(models.AppNotFound),
		}, fmt.Errorf("app not found: %s", u.packageName)
	}
	time.Sleep(3 * time.Second)

	// 5. 登录检测
	if err := u.HandleLoginIfNeeded(60); err != nil {
		return &models.CheckinResult{
			Success:     false,
			Action:      string(action),
			Message:     "登录超时: " + err.Error(),
			Timestamp:   timestamp,
			FailureCode: string(models.LoginTimeout),
		}, err
	}

	// 6. 导航到考勤页面
	navOk, recovery := u.navigator.NavigateToCheckin()
	if !navOk {
		return &models.CheckinResult{
			Success:        false,
			Action:         string(action),
			Message:        "导航到考勤页面失败",
			Timestamp:      timestamp,
			FailureCode:    string(models.NavigationFailed),
			RecoveryAction: recovery,
		}, fmt.Errorf("navigation failed")
	}

	// 7. 查找并点击打卡按钮（三策略）
	slog.Info("查找打卡按钮", "text", actionText)
	found, findErr := FindAndClickButton(action, u)
	if !found {
		msg := "未找到打卡按钮"
		if findErr != nil {
			msg = findErr.Error()
		}
		// 已打卡错误 → 特殊处理
		if ae, ok := findErr.(*AlreadyCheckedInError); ok {
			if ae.InCorrectSlot {
				return &models.CheckinResult{
					Success: true,
					Action:  string(action),
					Message: fmt.Sprintf("该时段已打卡 (%s)", ae.CheckinTime),
					Timestamp: timestamp,
				}, nil
			}
			return &models.CheckinResult{
				Success:     false,
				Action:      string(action),
				Message:     msg,
				Timestamp:   timestamp,
				FailureCode: string(models.AlreadyCheckedIn),
			}, findErr
		}
		return &models.CheckinResult{
			Success:     false,
			Action:      string(action),
			Message:     msg,
			Timestamp:   timestamp,
			FailureCode: string(models.ButtonNotFound),
		}, findErr
	}
	time.Sleep(2 * time.Second)

	// 8. 处理确认弹窗
	if err := u.verifier.HandleConfirmDialog(30); err != nil {
		if chkErr, ok := err.(*CheckinError); ok {
			return &models.CheckinResult{
				Success:        false,
				Action:         string(action),
				Message:        chkErr.Message,
				Timestamp:      timestamp,
				FailureCode:    chkErr.FailureCode,
				RecoveryAction: u.navigator.LastRecoveryAction(),
			}, chkErr
		}
	}

	// 9. 验证打卡结果
	success := u.verifier.DefaultVerify(action)
	message := "打卡成功"
	failureCode := ""
	if !success {
		message = "打卡失败：无法确认结果"
		failureCode = string(models.CheckinFailed)
		screenshotFn := func() ([]byte, error) { return u.Screenshot() }
		xmlFn := func() (string, error) { return u.DumpHierarchy() }
		utils.SaveFailureDiagnosis(u.baseDir, string(action), screenshotFn, xmlFn)
	}

	// 10. 返回工作台
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

// DeviceConnectionError 设备连接错误
type DeviceConnectionError struct {
	Message string
}

func (e *DeviceConnectionError) Error() string { return e.Message }
