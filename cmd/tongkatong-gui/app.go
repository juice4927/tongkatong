package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/automator"
	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/holiday"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/utils"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App Wails 后端应用
type App struct {
	mu              sync.Mutex
	ctx             context.Context
	configManager   *config.ConfigManager
	baseDir         string
	adbHelper       *adb.ADBHelper
	devicePool      *adb.DevicePool
	mumuHelper      *adb.MuMuHelper
	automator       *automator.UIAutomator2Impl
	holidayChecker  *holiday.HolidayChecker
	orchestrator    *automator.CheckinOrchestrator

	isConnected     bool
	isRunning       bool
}

// NewApp 创建 App 实例
func NewApp(cm *config.ConfigManager, baseDir string) *App {
	return &App{
		configManager: cm,
		baseDir:       baseDir,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// 注册日志回调 → Wails EventsEmit → 前端 EventsOn("log", ...)
	lm := utils.GetLogManager()
	lm.AddCallback(func(msg, level string) {
		runtime.EventsEmit(ctx, "log", map[string]string{
			"message": msg,
			"level":   level,
		})
	})

	slog.Info("GUI 启动完成")
}

func (a *App) shutdown(ctx context.Context) {
	slog.Info("GUI 正在退出...")
	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	if a.automator != nil {
		a.automator.Disconnect()
	}
	utils.GetLogManager().Close()
	_ = ctx
}

// ── 获取信息 ──────────────────────────────────────────────────────

// GetVersionInfo 返回版本信息
func (a *App) GetVersionInfo() map[string]string {
	return map[string]string{
		"app_name":   models.AppName,
		"version":    models.Version,
		"build_date": models.BuildDate,
	}
}

// GetConfigJSON 返回配置 JSON
func (a *App) GetConfigJSON() string {
	cfg := a.configManager.Config()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	return string(data)
}

// GetStatus 返回当前状态
func (a *App) GetStatus() map[string]interface{} {
	a.mu.Lock()
	defer a.mu.Unlock()

	devices := []map[string]string{}
	if a.adbHelper != nil {
		for _, d := range a.adbHelper.Devices() {
			devices = append(devices, map[string]string{
				"serial": d.Serial,
				"status": d.Status,
			})
		}
	}

	return map[string]interface{}{
		"is_connected": a.isConnected,
		"is_running":   a.isRunning,
		"devices":      devices,
	}
}

// GetCheckinTimes 返回今天打卡时间配置
func (a *App) GetCheckinTimes() map[string]interface{} {
	cfg := a.configManager.Config()
	times := cfg.Checkin
	result := make(map[string]interface{})
	for k, v := range times {
		result[k] = map[string]interface{}{
			"enabled":    v.Enabled,
			"time_range": v.TimeRange,
			"label":      v.Label,
		}
	}
	return result
}

// ── 操作 ──────────────────────────────────────────────────────────

// ConnectDevice 连接设备（匹配 Python 版逻辑：多端口尝试 + 自动启动 MuMu）
func (a *App) ConnectDevice() string {
	cfg := a.configManager.Config()

	// 初始化 ADB（自动查找 MuMu 自带的 adb）
	a.adbHelper = adb.NewADBHelper(cfg.MuMu.AdbPath)
	a.mumuHelper = adb.NewMuMuHelper(cfg.MuMu.AdbPath, cfg.MuMu.MuMuExePath)
	a.devicePool = adb.NewDevicePool(a.adbHelper, time.Duration(cfg.Advanced.SessionTTLSeconds)*time.Second)

	foundPath := a.mumuHelper.FindAdb()
	if foundPath != "" && foundPath != "adb" && foundPath != cfg.MuMu.AdbPath {
		slog.Info("自动发现 MuMu ADB", "path", foundPath)
		a.adbHelper.SetADBPath(foundPath)
		a.mumuHelper = adb.NewMuMuHelper(foundPath, cfg.MuMu.MuMuExePath)
	}

	// 端口候选项（匹配 Python 版：配置端口 → 7555 → 5555）
	ports := uniquePorts(cfg.MuMu.Port)

	// 第一阶段：直接尝试连接各端口
	for _, port := range ports {
		ok, msg := a.adbHelper.Connect(cfg.MuMu.Host, port)
		if ok {
			a.isConnected = true
			a.initEngine(cfg)
			// 用实际连接的端口更新配置
			cfg.MuMu.Port = port
			_ = a.configManager.SaveConfig(cfg)
			return fmt.Sprintf("连接成功: %s (端口 %d)", msg, port)
		}
		slog.Info("端口连接失败，尝试下一个", "port", port, "msg", msg)
	}

	// 第二阶段：所有端口失败 → 尝试启动 MuMu
	slog.Info("所有端口连接失败，尝试启动 MuMu 模拟器")
	launched, launchMsg := a.mumuHelper.LaunchMuMu(a.adbHelper, cfg.MuMu.Host, cfg.MuMu.Port, 60)
	if !launched {
		return "连接失败: " + launchMsg
	}

	// MuMu 已启动，重新尝试所有端口
	for _, port := range ports {
		ok, msg := a.adbHelper.Connect(cfg.MuMu.Host, port)
		if ok {
			a.isConnected = true
			a.initEngine(cfg)
			cfg.MuMu.Port = port
			_ = a.configManager.SaveConfig(cfg)
			return fmt.Sprintf("连接成功: %s (MuMu 已自动启动, 端口 %d)", msg, port)
		}
	}

	return fmt.Sprintf("连接失败: MuMu 已启动但 ADB 连接超时 (已尝试端口: %v)", ports)
}

func uniquePorts(configured int) []int {
	seen := map[int]bool{}
	var result []int
	for _, p := range []int{configured, 7555, 5555} {
		if p > 0 && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}
	return result
}

func (a *App) initEngine(cfg *config.Config) {
	a.automator = automator.NewUIAutomator2Impl(
		cfg.MuMu.Host, cfg.MuMu.Port,
		a.adbHelper.GetADBPath(),
		cfg.App.PackageName,
		a.devicePool,
	)
	a.automator.SetMuMuHelper(a.mumuHelper)

	a.holidayChecker = holiday.NewHolidayChecker(
		cfg.Holiday.SkipWeekend,
		cfg.Holiday.SkipHoliday,
		cfg.Holiday.ExtraWorkdays,
		cfg.Holiday.ExtraHolidays,
	)
	holidayCachePath := filepath.Join(a.baseDir, "holidays.json")
	a.holidayChecker.SetLocalCachePath(holidayCachePath)
}

// DisconnectDevice 断开设备
func (a *App) DisconnectDevice() string {
	if a.automator != nil {
		a.automator.Disconnect()
	}
	a.isConnected = false
	return "已断开"
}

// StartScheduler 启动打卡调度
func (a *App) StartScheduler() string {
	if !a.isConnected {
		return "请先连接设备"
	}

	if a.isRunning {
		return "调度已在运行中"
	}

	if a.orchestrator == nil {
		a.orchestrator = automator.NewCheckinOrchestrator(
			a.automator, a.holidayChecker, a.configManager, a.baseDir,
		)
		if !a.orchestrator.Initialize() {
			return "调度器初始化失败"
		}
	}

	a.orchestrator.Start()
	a.isRunning = true
	return "调度已启动"
}

// StopScheduler 停止打卡调度
func (a *App) StopScheduler() string {
	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	a.isRunning = false
	return "调度已停止"
}

// ManualCheckin 手动执行打卡
func (a *App) ManualCheckin(action string) string {
	if !a.isConnected || a.automator == nil {
		return "请先连接设备"
	}

	var checkinAction automator.CheckinAction
	switch action {
	case "morning_signin":
		checkinAction = automator.MorningSignin
	case "morning_signout":
		checkinAction = automator.MorningSignout
	case "afternoon_signin":
		checkinAction = automator.AfternoonSignin
	case "afternoon_signout":
		checkinAction = automator.AfternoonSignout
	default:
		return "无效的打卡类型"
	}

	result, err := a.automator.DoCheckin(checkinAction)
	if err != nil {
		return "打卡异常: " + err.Error()
	}

	status := "成功"
	if !result.Success {
		status = "失败"
	}
	return "打卡" + status + ": " + result.Message
}

// CheckHoliday 判断今天是否工作日
func (a *App) CheckHoliday(dateStr string) map[string]interface{} {
	date := time.Now()
	if dateStr != "" {
		parsed, err := time.Parse("2006-01-02", dateStr)
		if err == nil {
			date = parsed
		}
	}

	isWorkday := true
	holidayName := ""
	if a.holidayChecker != nil {
		isWorkday = a.holidayChecker.IsWorkday(date)
		holidayName = a.holidayChecker.GetHolidayName(date)
	} else {
		// 降级判断
		weekday := date.Weekday()
		isWorkday = weekday != time.Saturday && weekday != time.Sunday
	}

	return map[string]interface{}{
		"date":         date.Format("2006-01-02"),
		"is_workday":   isWorkday,
		"holiday_name": holidayName,
	}
}

// GetDailyResults 获取当天打卡结果
func (a *App) GetDailyResults() []map[string]interface{} {
	if a.orchestrator == nil {
		return []map[string]interface{}{}
	}

	records := a.orchestrator.GetDailyResults()
	result := make([]map[string]interface{}, len(records))
	for i, r := range records {
		result[i] = map[string]interface{}{
			"action_name": r.ActionName,
			"success":     r.Success,
			"message":     r.Message,
			"timestamp":   r.Timestamp,
		}
	}
	return result
}

// SaveConfig 保存配置并重载
func (a *App) SaveConfig(jsonStr string) string {
	var newCfg config.Config
	if err := json.Unmarshal([]byte(jsonStr), &newCfg); err != nil {
		return "配置解析失败: " + err.Error()
	}

	if err := a.configManager.SaveConfig(&newCfg); err != nil {
		return "配置保存失败: " + err.Error()
	}

	slog.Info("配置已保存并重载")
	return "配置已保存"
}

// ExportConfig 导出配置
func (a *App) ExportConfig() string {
	data, _ := json.MarshalIndent(a.configManager.Config(), "", "  ")
	return string(data)
}

// ImportConfig 导入配置
func (a *App) ImportConfig(jsonStr string) string {
	return a.SaveConfig(jsonStr)
}

// GetAvailablePackages 获取设备上已安装的应用包名
func (a *App) GetAvailablePackages() []string {
	if a.adbHelper == nil {
		return nil
	}
	// 使用当前连接的设备（adb shell 默认选唯一设备，多设备时需指定）
	serial := ""
	devices := a.adbHelper.Devices()
	for _, d := range devices {
		if d.Status == "device" {
			serial = d.Serial
			break
		}
	}
	ok, out := a.adbHelper.Shell(serial, "pm list packages 2>/dev/null", 15*time.Second)
	if !ok {
		return nil
	}
	lines := strings.Split(out, "\n")
	packages := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 8 && line[:8] == "package:" {
			packages = append(packages, line[8:])
		}
	}
	return packages
}

// TestConnection 测试 ADB 连接
func (a *App) TestConnection() string {
	if a.adbHelper == nil {
		cfg := a.configManager.Config()
		a.adbHelper = adb.NewADBHelper(cfg.MuMu.AdbPath)
	}
	ok, msg := a.adbHelper.Connect(a.configManager.Config().MuMu.Host, a.configManager.Config().MuMu.Port)
	if ok {
		version := a.adbHelper.Version()
		a.adbHelper.Disconnect(a.configManager.Config().MuMu.Host, a.configManager.Config().MuMu.Port)
		return "连接成功 · ADB " + version + " · " + msg
	}
	return "连接失败: " + msg
}

// GetDefaultConfig 返回默认配置 JSON
func (a *App) GetDefaultConfig() string {
	return config.DefaultConfigJSON()
}

// CheckHolidayUpdate 检查并更新节假日数据
func (a *App) CheckHolidayUpdate() string {
	if a.holidayChecker == nil {
		return "请先连接设备"
	}
	ok := a.holidayChecker.TryUpdateFromRemote("https://raw.githubusercontent.com/juice4927/holiday-china/main/holidays.json")
	if ok {
		return "节假日数据已更新"
	}
	return "节假日数据已是最新或无网络"
}

// BrowseFile 打开原生文件选择器，返回选定文件路径（空串=取消）
func (a *App) BrowseFile() string {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 ADB 可执行文件",
		Filters: []runtime.FileFilter{
			{DisplayName: "可执行文件 (*.exe)", Pattern: "*.exe"},
			{DisplayName: "所有文件 (*)", Pattern: "*"},
		},
	})
	if err != nil || path == "" {
		return ""
	}
	return path
}

// BrowseDirectory 打开原生目录选择器，返回选定目录路径（空串=取消）
func (a *App) BrowseDirectory() string {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 MuMu 安装目录",
	})
	if err != nil || path == "" {
		return ""
	}
	return path
}

// GetLogContent 获取日志文件内容
func (a *App) GetLogContent() string {
	logPath := filepath.Join(a.baseDir, "..", "logs")
	today := time.Now().Format("2006-01-02")
	logFile := filepath.Join(logPath, "checkin_"+today+".log")

	data, err := os.ReadFile(logFile)
	if err != nil {
		return "日志文件不可用"
	}
	return string(data)
}
