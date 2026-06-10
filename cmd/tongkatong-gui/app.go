package main

import (
	"context"
	"encoding/json"
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

// ConnectDevice 连接设备
func (a *App) ConnectDevice() string {
	cfg := a.configManager.Config()

	a.adbHelper = adb.NewADBHelper(cfg.MuMu.AdbPath)
	a.devicePool = adb.NewDevicePool(a.adbHelper, time.Duration(cfg.Advanced.SessionTTLSeconds)*time.Second)
	a.mumuHelper = adb.NewMuMuHelper(cfg.MuMu.AdbPath, cfg.MuMu.MuMuExePath)

	// 自动查找 ADB
	foundPath := a.mumuHelper.FindAdb()
	if foundPath != cfg.MuMu.AdbPath && cfg.MuMu.AdbPath == "" {
		slog.Info("自动发现 MuMu ADB", "path", foundPath)
	}

	ok, msg := a.adbHelper.Connect(cfg.MuMu.Host, cfg.MuMu.Port)
	if ok {
		a.isConnected = true

		// 初始化自动化引擎
		a.automator = automator.NewUIAutomator2Impl(
			cfg.MuMu.Host, cfg.MuMu.Port,
			a.adbHelper.GetADBPath(),
			cfg.App.PackageName,
			a.devicePool,
		)
		a.automator.SetMuMuHelper(a.mumuHelper)

		// 初始化节假日判断
		a.holidayChecker = holiday.NewHolidayChecker(
			cfg.Holiday.SkipWeekend,
			cfg.Holiday.SkipHoliday,
			cfg.Holiday.ExtraWorkdays,
			cfg.Holiday.ExtraHolidays,
		)

		// 尝试更新节假日数据
		holidayCachePath := filepath.Join(a.baseDir, "holidays.json")
		a.holidayChecker.SetLocalCachePath(holidayCachePath)

		return "连接成功: " + msg
	}

	return "连接失败: " + msg
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
