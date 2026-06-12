package main

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/automator"
	"github.com/juice4927/tongkatong/internal/autostart"
	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/holiday"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/updater"
	"github.com/juice4927/tongkatong/internal/utils"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ── 恢复状态 ─────────────────────────────────────────────────

type recoveryState struct {
	mu             sync.Mutex
	inProgress     bool
	failCount      int
	nextRetryAt    time.Time
	startedAt      time.Time
	pausedUntil    time.Time
	lastAction     string
	lastReason     string
	lastResult     string
	lastError      string
	lastRecoveryAt time.Time
}

func (r *recoveryState) markFailed(reason string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failCount++
	r.inProgress = false
	r.lastReason = reason
	r.lastResult = "failed"
	r.lastError = reason
	r.lastRecoveryAt = time.Now()
}

func (r *recoveryState) markSucceeded() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failCount = 0
	r.nextRetryAt = time.Time{}
	r.pausedUntil = time.Time{}
	r.lastResult = "success"
	r.lastReason = ""
	r.lastError = ""
	r.lastRecoveryAt = time.Now()
	r.inProgress = false
}

func (r *recoveryState) snapshot() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := ""
	if !r.nextRetryAt.IsZero() {
		next = r.nextRetryAt.Format("2006-01-02 15:04:05")
	}
	paused := ""
	if !r.pausedUntil.IsZero() {
		paused = r.pausedUntil.Format("2006-01-02 15:04:05")
	}
	lastRecoveryAt := ""
	if !r.lastRecoveryAt.IsZero() {
		lastRecoveryAt = r.lastRecoveryAt.Format("2006-01-02 15:04:05")
	}
	return map[string]interface{}{
		"fail_count":       r.failCount,
		"in_progress":      r.inProgress,
		"next_retry_at":    next,
		"paused_until":     paused,
		"last_action":      r.lastAction,
		"last_reason":      r.lastReason,
		"last_result":      r.lastResult,
		"last_error":       r.lastError,
		"last_recovery_at": lastRecoveryAt,
	}
}

// App Wails 后端应用
type App struct {
	mu             sync.Mutex
	ctx            context.Context
	configManager  *config.ConfigManager
	baseDir        string
	adbHelper      *adb.ADBHelper
	devicePool     *adb.DevicePool
	mumuHelper     *adb.MuMuHelper
	automator      *automator.UIAutomator2Impl
	holidayChecker *holiday.HolidayChecker
	orchestrator   *automator.CheckinOrchestrator

	isConnected     bool
	isRunning       bool
	connecting      bool
	connectAttempt  uint64
	desiredRunning  bool // 用户意图：是否希望保持运行状态
	pendingRecover  bool // 恢复流程挂起标记
	quitting        bool
	lastPrecheckKey string

	recovery *recoveryState

	guardStopCh   chan struct{} // 守护协程退出信号
	guardStopOnce sync.Once
	tray          trayController
}

// NewApp 创建 App 实例
func NewApp(cm *config.ConfigManager, baseDir string) *App {
	app := &App{
		configManager: cm,
		baseDir:       baseDir,
		recovery:      &recoveryState{},
	}
	app.tray = newTrayController(app)
	return app
}

func (a *App) stopGuard() {
	if a.guardStopCh == nil {
		return
	}
	a.guardStopOnce.Do(func() {
		close(a.guardStopCh)
	})
}

func (a *App) startTray() {
	if a.tray != nil {
		a.tray.Start()
	}
}

func (a *App) stopTray() {
	if a.tray != nil {
		a.tray.Stop()
	}
}

func (a *App) refreshTray() {
	if a.tray != nil {
		a.tray.Refresh()
	}
}

func (a *App) trayEnabled() bool {
	return a.tray != nil && a.tray.Enabled()
}

func (a *App) trayStatus() (bool, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.isConnected, a.isRunning
}

func (a *App) syncBootAutoStartState() {
	cfg := a.configManager.Config()
	cfg.AppState.BootAutoStart = autostart.IsEnabled()
}

func (a *App) beginConnectAttempt() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.connectAttempt++
	a.connecting = true
	return a.connectAttempt
}

func (a *App) isConnectAttemptActive(attempt uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.connecting && a.connectAttempt == attempt
}

func (a *App) cancelConnectAttempt() {
	a.mu.Lock()
	a.connectAttempt++
	a.connecting = false
	a.isConnected = false
	a.mu.Unlock()
}

func (a *App) clearDisconnectedState() {
	a.mu.Lock()
	a.isConnected = false
	a.isRunning = false
	a.desiredRunning = false
	a.pendingRecover = false
	a.mu.Unlock()
}

func (a *App) commitConnectAttempt(attempt uint64) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.connecting || a.connectAttempt != attempt {
		return false
	}
	a.connecting = false
	return true
}

func (a *App) emitGuardStatus() {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "guard_status", a.GetGuardStatus())
}

func (a *App) emitUpdateStatus(status, detail string, available bool, latest string) {
	if a.ctx == nil {
		return
	}
	runtime.EventsEmit(a.ctx, "update_status", map[string]interface{}{
		"status":    status,
		"detail":    detail,
		"available": available,
		"latest":    latest,
	})
}

func (a *App) emitUpdateProgress(downloaded, total int64) {
	if a.ctx == nil {
		return
	}
	progress := 0
	if total > 0 {
		progress = int((downloaded * 100) / total)
	}
	runtime.EventsEmit(a.ctx, "update_progress", map[string]interface{}{
		"downloaded": downloaded,
		"total":      total,
		"progress":   progress,
	})
}

func inQuietHours(state config.AppStateConfig, now time.Time) bool {
	if !state.RecoveryQuietHoursEnabled {
		return false
	}
	start := state.RecoveryQuietStartHour
	end := state.RecoveryQuietEndHour
	hour := now.Hour()
	if start == end {
		return true
	}
	if start < end {
		return hour >= start && hour < end
	}
	return hour >= start || hour < end
}

func (a *App) scheduleRecoveryFailure(action, reason string, cfg *config.Config) {
	now := time.Now()
	base := cfg.AppState.RecoveryBaseBackoffSeconds
	if base <= 0 {
		base = 5
	}
	maxBackoff := cfg.AppState.RecoveryMaxBackoffSeconds
	if maxBackoff < base {
		maxBackoff = base
	}

	a.recovery.mu.Lock()
	defer a.recovery.mu.Unlock()

	a.recovery.failCount++
	a.recovery.inProgress = false
	a.recovery.lastAction = action
	a.recovery.lastReason = reason
	a.recovery.lastResult = "failed"
	a.recovery.lastError = reason
	a.recovery.lastRecoveryAt = now

	backoff := base
	for i := 1; i < a.recovery.failCount && backoff < maxBackoff; i++ {
		if backoff > maxBackoff/2 {
			backoff = maxBackoff
			break
		}
		backoff *= 2
	}
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	a.recovery.nextRetryAt = now.Add(time.Duration(backoff) * time.Second)
	a.recovery.pausedUntil = time.Time{}

	maxFailures := cfg.AppState.RecoveryMaxFailures
	if maxFailures > 0 && a.recovery.failCount >= maxFailures {
		pauseMinutes := cfg.AppState.RecoveryPauseMinutesAfterMax
		if pauseMinutes <= 0 {
			pauseMinutes = 30
		}
		a.recovery.pausedUntil = now.Add(time.Duration(pauseMinutes) * time.Minute)
		a.recovery.nextRetryAt = time.Time{}
		a.recovery.lastResult = "paused"
		a.recovery.lastError = fmt.Sprintf("%s；已达到失败阈值，暂停恢复", reason)
	}
}

func (a *App) autoCheckForUpdates() {
	cfg := a.configManager.Config()
	if !cfg.Update.AutoCheckOnStartup {
		return
	}

	url := cfg.Update.ManifestURL
	if url == "" {
		url = "https://raw.githubusercontent.com/juice4927/tongkatong-update/main/version.json"
	}

	a.emitUpdateStatus("正在检查", "启动后自动检查更新中...", false, "")

	asset, needsUpdate, err := updater.CheckUpdate(url, models.Version, "default")
	if err != nil {
		a.emitUpdateStatus("检查失败", err.Error(), false, "")
		return
	}
	if !needsUpdate {
		a.emitUpdateStatus("已是最新", "当前版本已是最新版本", false, models.Version)
		return
	}

	a.emitUpdateStatus("发现新版本", fmt.Sprintf("发现 v%s，正在后台预下载", asset.Version), true, asset.Version)
	ok, destPath, err := updater.SilentCheckAndPreDownload(url, models.Version, "default")
	if err != nil {
		a.emitUpdateStatus("发现新版本", fmt.Sprintf("发现 v%s，但预下载失败：%v", asset.Version, err), true, asset.Version)
		return
	}
	if ok {
		a.emitUpdateStatus("发现新版本", fmt.Sprintf("v%s 已预下载到 %s", asset.Version, filepath.Base(destPath)), true, asset.Version)
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.syncBootAutoStartState()

	// 注册日志回调 → Wails EventsEmit → 前端 EventsOn("log", ...)
	lm := utils.GetLogManager()
	lm.AddCallback(func(msg, level string) {
		runtime.EventsEmit(ctx, "log", map[string]string{
			"message": msg,
			"level":   level,
		})
	})

	slog.Info("GUI 启动完成")
	a.startTray()

	// 启动守护协程（每15秒检查连接/运行状态）
	a.guardStopOnce = sync.Once{}
	a.guardStopCh = make(chan struct{})
	go a.keepAliveGuard()

	cfg := a.configManager.Config()
	if cfg.Update.AutoCheckOnStartup {
		go func() {
			time.Sleep(1200 * time.Millisecond)
			a.autoCheckForUpdates()
		}()
	}

	// 自动连接
	if cfg.AppState.AutoConnect {
		go func() {
			time.Sleep(500 * time.Millisecond) // 等窗口完全渲染
			a.ConnectDevice()
			if cfg.AppState.AutoStart && a.isConnected {
				a.StartScheduler()
			}
		}()
	}
}

func (a *App) shutdown(ctx context.Context) {
	slog.Info("GUI 正在退出...")
	// 停止守护协程
	a.stopGuard()
	a.stopTray()
	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	if a.automator != nil {
		a.automator.Disconnect()
	}
	utils.GetLogManager().Close()
}

func (a *App) domReady(ctx context.Context) {
	slog.Info("前端DOM就绪")
}

// OnBeforeClose 关闭窗口时检查是否最小化到托盘
func (a *App) onBeforeClose(ctx context.Context) bool {
	a.mu.Lock()
	quitting := a.quitting
	desired := a.desiredRunning
	running := a.isRunning
	a.mu.Unlock()

	if quitting {
		return true
	}

	if a.trayEnabled() {
		slog.Info("关闭窗口时隐藏到系统托盘")
		runtime.WindowHide(ctx)
		return false
	}

	// 如果正在运行中，最小化到托盘而不是退出
	if running || desired {
		slog.Info("正在运行中，最小化到托盘")
		runtime.WindowHide(ctx)
		return false // 阻止关闭
	}
	return true // 允许关闭
}

// Quit 强制退出（停止一切并关闭）
func (a *App) Quit() string {
	slog.Info("用户请求退出...")
	a.mu.Lock()
	a.desiredRunning = false
	a.isRunning = false
	a.isConnected = false
	a.quitting = true
	a.mu.Unlock()

	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	if a.automator != nil {
		a.automator.Disconnect()
	}
	a.stopGuard()
	a.stopTray()

	// 延迟退出让日志写完；若 Wails 退出被托盘或窗口状态阻塞，最终强制结束进程。
	go func() {
		time.Sleep(150 * time.Millisecond)
		if a.ctx != nil {
			runtime.Quit(a.ctx)
		}
		time.Sleep(1200 * time.Millisecond)
		os.Exit(0)
	}()
	return "正在退出..."
}

// HideWindow 手动隐藏窗口到托盘
func (a *App) HideWindow() string {
	runtime.WindowHide(a.ctx)
	a.refreshTray()
	return "已最小化到托盘"
}

// ShowWindow 从托盘恢复窗口
func (a *App) ShowWindow() string {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	a.refreshTray()
	return "窗口已恢复"
}

// ── 守护逻辑 ─────────────────────────────────────────────────

func (a *App) keepAliveGuard() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-a.guardStopCh:
			return
		case <-ticker.C:
			a.guardTick()
		}
	}
}

func (a *App) guardTick() {
	a.mu.Lock()
	connected := a.isConnected
	running := a.isRunning
	desired := a.desiredRunning
	a.mu.Unlock()

	cfg := a.configManager.Config()
	if !cfg.AppState.KeepAliveEnabled {
		a.emitGuardStatus()
		return
	}
	a.runPreCheckinGuard(time.Now(), cfg, connected, running, desired)

	if !desired || (connected && running) {
		a.emitGuardStatus()
		return
	}

	now := time.Now()

	a.recovery.mu.Lock()
	if a.recovery.inProgress {
		a.recovery.mu.Unlock()
		a.emitGuardStatus()
		return
	}
	if inQuietHours(cfg.AppState, now) {
		a.recovery.lastAction = "quiet_hours"
		a.recovery.lastResult = "waiting"
		a.recovery.lastReason = "静默时段内暂停自动恢复"
		a.recovery.lastError = ""
		a.recovery.inProgress = false
		a.recovery.mu.Unlock()
		a.emitGuardStatus()
		return
	}
	if !a.recovery.pausedUntil.IsZero() && now.Before(a.recovery.pausedUntil) {
		a.recovery.lastAction = "pause"
		a.recovery.lastResult = "paused"
		a.recovery.lastReason = "恢复次数达到阈值，当前处于暂停期"
		a.recovery.lastError = ""
		a.recovery.inProgress = false
		a.recovery.mu.Unlock()
		a.emitGuardStatus()
		return
	}
	if !a.recovery.nextRetryAt.IsZero() && now.Before(a.recovery.nextRetryAt) {
		a.recovery.lastResult = "waiting"
		a.recovery.inProgress = false
		a.recovery.mu.Unlock()
		a.emitGuardStatus()
		return
	}

	action := "restart_scheduler"
	if !connected {
		action = "reconnect"
	}
	a.recovery.inProgress = true
	a.recovery.lastAction = action
	a.recovery.lastResult = "running"
	a.recovery.lastReason = ""
	a.recovery.lastError = ""
	a.recovery.lastRecoveryAt = now
	a.recovery.mu.Unlock()

	if !connected {
		slog.Info("守护：检测到断开，尝试恢复连接")
		attempt := a.beginConnectAttempt()
		msg := a.connectDeviceSync(attempt)
		if !a.commitConnectAttempt(attempt) {
			msg = "连接已取消"
		}
		a.mu.Lock()
		reconnected := a.isConnected
		restarted := a.isRunning
		a.mu.Unlock()
		if reconnected && restarted {
			a.emitGuardStatus()
			a.emitStatus()
			return
		}
		a.scheduleRecoveryFailure(action, msg, cfg)
		a.emitGuardStatus()
		return
	}

	slog.Info("守护：调度未运行，尝试重启")
	msg := a.StartScheduler()
	a.mu.Lock()
	restarted := a.isRunning
	a.mu.Unlock()
	if restarted {
		a.emitGuardStatus()
		a.emitStatus()
		return
	}
	a.scheduleRecoveryFailure(action, msg, cfg)
	a.emitGuardStatus()
}

func (a *App) runPreCheckinGuard(now time.Time, cfg *config.Config, connected, running, desired bool) {
	if !desired || a.orchestrator == nil {
		return
	}

	jobID, label, runAt, found := a.nextUpcomingCheckinWithin(now, 10*time.Minute)
	if !found {
		return
	}

	issues := a.collectRuntimeIssues(connected, running)
	if len(issues) == 0 {
		return
	}

	windowKey := jobID + "|" + runAt.Format(time.RFC3339)
	a.mu.Lock()
	if a.lastPrecheckKey == windowKey {
		a.mu.Unlock()
		return
	}
	a.lastPrecheckKey = windowKey
	a.mu.Unlock()

	minutesLeft := int(time.Until(runAt).Minutes())
	if minutesLeft < 0 {
		minutesLeft = 0
	}
	problem := strings.Join(issues, "、")
	slog.Warn("打卡前预警检查触发", "checkin", label, "run_at", runAt.Format("15:04"), "issues", problem, "minutes_left", minutesLeft)

	action := "precheck_restart_scheduler"
	if !connected {
		action = "precheck_reconnect"
	}
	if a.beginRecovery(action, "距打卡不足10分钟专项恢复", now) {
		ok, msg := a.attemptRecovery(action, cfg)
		if ok {
			slog.Info("打卡前预警恢复成功", "checkin", label, "message", msg)
			a.emitGuardStatus()
			a.emitStatus()
		} else {
			slog.Warn("打卡前预警恢复未完成", "checkin", label, "message", msg)
			a.emitGuardStatus()
		}
	}

	time.AfterFunc(30*time.Second, func() {
		a.checkPrecheckRecoveryResult(windowKey, label, runAt, minutesLeft)
	})
}

func (a *App) nextUpcomingCheckinWithin(now time.Time, window time.Duration) (string, string, time.Time, bool) {
	if a.orchestrator == nil {
		return "", "", time.Time{}, false
	}

	cfg := a.configManager.Config()
	var (
		bestJobID string
		bestLabel string
		bestTime  time.Time
	)
	for jobID, runAt := range a.orchestrator.GetScheduledCheckinTimes() {
		if !runAt.After(now) {
			continue
		}
		if runAt.Sub(now) > window {
			continue
		}
		if bestTime.IsZero() || runAt.Before(bestTime) {
			bestTime = runAt
			bestJobID = jobID
			bestLabel = jobID
			if entry, ok := cfg.Checkin[jobID]; ok && entry.Label != "" {
				bestLabel = entry.Label
			}
		}
	}
	if bestTime.IsZero() {
		return "", "", time.Time{}, false
	}
	return bestJobID, bestLabel, bestTime, true
}

func (a *App) collectRuntimeIssues(connected, running bool) []string {
	issues := make([]string, 0, 2)
	if !connected {
		issues = append(issues, "设备未连接")
	}
	if !running {
		issues = append(issues, "打卡未启动")
	}
	return issues
}

func (a *App) beginRecovery(action, reason string, now time.Time) bool {
	a.recovery.mu.Lock()
	defer a.recovery.mu.Unlock()
	if a.recovery.inProgress {
		return false
	}
	a.recovery.inProgress = true
	a.recovery.lastAction = action
	a.recovery.lastResult = "running"
	a.recovery.lastReason = reason
	a.recovery.lastError = ""
	a.recovery.lastRecoveryAt = now
	return true
}

func (a *App) attemptRecovery(action string, cfg *config.Config) (bool, string) {
	a.mu.Lock()
	connected := a.isConnected
	a.mu.Unlock()

	if !connected {
		attempt := a.beginConnectAttempt()
		msg := a.connectDeviceSync(attempt)
		if !a.commitConnectAttempt(attempt) {
			msg = "连接已取消"
		}
		a.mu.Lock()
		reconnected := a.isConnected
		restarted := a.isRunning
		a.mu.Unlock()
		if reconnected && restarted {
			return true, msg
		}
		a.scheduleRecoveryFailure(action, msg, cfg)
		return false, msg
	}

	msg := a.StartScheduler()
	a.mu.Lock()
	restarted := a.isRunning
	a.mu.Unlock()
	if restarted {
		return true, msg
	}
	a.scheduleRecoveryFailure(action, msg, cfg)
	return false, msg
}

func (a *App) checkPrecheckRecoveryResult(windowKey, label string, runAt time.Time, minutesLeft int) {
	a.mu.Lock()
	connected := a.isConnected
	running := a.isRunning
	desired := a.desiredRunning
	currentKey := a.lastPrecheckKey
	a.mu.Unlock()

	cfg := a.configManager.Config()
	if !cfg.AppState.KeepAliveEnabled || !desired || currentKey != windowKey {
		return
	}

	issues := a.collectRuntimeIssues(connected, running)
	if len(issues) == 0 {
		return
	}

	problem := strings.Join(issues, "、")
	slog.Warn("打卡前预警：自动恢复失败", "checkin", label, "issues", problem)

	if !cfg.Notification.Enabled || strings.TrimSpace(cfg.Notification.Webhook) == "" {
		return
	}

	title := "通卡通预警 - " + problem
	body := fmt.Sprintf("距 %s 打卡仅剩 %d 分钟，自动恢复失败，请手动检查。\n\n**目标打卡**：%s\n\n**问题**：%s", runAt.Format("15:04"), minutesLeft, label, problem)
	go utils.SendServerChan(cfg.Notification.Webhook, title, body, cfg.Notification.VerifyTLS)
}

// GetGuardStatus 返回守护状态快照
func (a *App) GetGuardStatus() map[string]interface{} {
	a.mu.Lock()
	connected := a.isConnected
	running := a.isRunning
	desired := a.desiredRunning
	a.mu.Unlock()

	cfg := a.configManager.Config()
	snap := a.recovery.snapshot()
	snap["keep_alive_enabled"] = cfg.AppState.KeepAliveEnabled
	snap["is_connected"] = connected
	snap["is_running"] = running
	snap["desired_running"] = desired
	return snap
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
		"is_connected":    a.isConnected,
		"is_running":      a.isRunning,
		"desired_running": a.desiredRunning,
		"devices":         devices,
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

// ── 操作（异步执行，通过事件推结果到前端）─────────────────────

// ConnectDeviceAsync 异步连接设备（不阻塞UI，通过事件推送结果）
func (a *App) ConnectDeviceAsync() string {
	attempt := a.beginConnectAttempt()
	go func() {
		msg := a.connectDeviceSync(attempt)
		if !a.commitConnectAttempt(attempt) {
			slog.Info("忽略已取消的连接结果", "attempt", attempt, "message", msg)
			return
		}
		runtime.EventsEmit(a.ctx, "connect_result", map[string]interface{}{
			"success": a.isConnected,
			"message": msg,
		})
		// 刷新状态推送到前端
		time.Sleep(200 * time.Millisecond)
		a.emitStatus()
	}()
	return "connecting"
}

// connectDeviceSync 同步连接设备（在 goroutine 中执行）
func (a *App) connectDeviceSync(attempt uint64) string {
	cfg := a.configManager.Config()

	a.adbHelper = adb.NewADBHelper(cfg.MuMu.AdbPath)
	a.mumuHelper = adb.NewMuMuHelper(cfg.MuMu.AdbPath, cfg.MuMu.MuMuExePath)
	a.devicePool = adb.NewDevicePool(a.adbHelper, time.Duration(cfg.Advanced.SessionTTLSeconds)*time.Second)

	foundPath := a.mumuHelper.FindAdb()
	if foundPath != "" && foundPath != "adb" && foundPath != cfg.MuMu.AdbPath {
		slog.Info("自动发现 MuMu ADB", "path", foundPath)
		a.adbHelper.SetADBPath(foundPath)
		a.mumuHelper = adb.NewMuMuHelper(foundPath, cfg.MuMu.MuMuExePath)
	}

	runtime.EventsEmit(a.ctx, "connect_progress", map[string]interface{}{"step": "trying_ports"})

	ports := uniquePorts(cfg.MuMu.Port)
	for _, port := range ports {
		if !a.isConnectAttemptActive(attempt) {
			return "连接已取消"
		}
		ok, msg := a.adbHelper.Connect(cfg.MuMu.Host, port)
		if ok {
			if !a.isConnectAttemptActive(attempt) {
				return "连接已取消"
			}
			a.mu.Lock()
			a.isConnected = true
			a.mu.Unlock()
			a.initEngine(cfg)
			cfg.MuMu.Port = port
			_ = a.configManager.SaveConfig(cfg)
			loginErr := a.postConnect(cfg, false)
			if !a.isConnectAttemptActive(attempt) {
				if a.automator != nil {
					a.automator.Disconnect()
				}
				a.mu.Lock()
				a.isConnected = false
				a.mu.Unlock()
				return "连接已取消"
			}
			a.emitGuardStatus()
			if loginErr != nil {
				return fmt.Sprintf("连接成功，但登录未完成: %v", loginErr)
			}
			return fmt.Sprintf("连接成功 (%s, 端口 %d)", msg, port)
		}
	}

	runtime.EventsEmit(a.ctx, "connect_progress", map[string]interface{}{"step": "launching_mumu"})
	launched, launchMsg := a.mumuHelper.LaunchMuMu(a.adbHelper, cfg.MuMu.Host, cfg.MuMu.Port, 60)
	if !launched {
		return "连接失败: " + launchMsg
	}

	for _, port := range ports {
		if !a.isConnectAttemptActive(attempt) {
			return "连接已取消"
		}
		ok, _ := a.adbHelper.Connect(cfg.MuMu.Host, port)
		if ok {
			if !a.isConnectAttemptActive(attempt) {
				return "连接已取消"
			}
			a.mu.Lock()
			a.isConnected = true
			a.mu.Unlock()
			a.initEngine(cfg)
			cfg.MuMu.Port = port
			_ = a.configManager.SaveConfig(cfg)
			loginErr := a.postConnect(cfg, true)
			if !a.isConnectAttemptActive(attempt) {
				if a.automator != nil {
					a.automator.Disconnect()
				}
				a.mu.Lock()
				a.isConnected = false
				a.mu.Unlock()
				return "连接已取消"
			}
			a.emitGuardStatus()
			if loginErr != nil {
				return fmt.Sprintf("连接成功，但登录未完成: %v", loginErr)
			}
			return fmt.Sprintf("连接成功，MuMu已自动启动 (端口 %d)", port)
		}
	}
	return "连接失败: 所有端口不可达"
}

// emitStatus 推送当前状态到前端
func (a *App) emitStatus() {
	runtime.EventsEmit(a.ctx, "status", a.GetStatus())
	runtime.EventsEmit(a.ctx, "dashboard", a.getDashboardData())
	a.refreshTray()
}

// getDashboardData 获取仪表盘数据
func (a *App) getDashboardData() map[string]interface{} {
	a.mu.Lock()
	connected := a.isConnected
	running := a.isRunning
	a.mu.Unlock()

	cfg := a.configManager.Config()
	results := a.GetDailyResults()
	hi := a.CheckHoliday("")
	times := a.GetCheckinTimes()
	plannedTimes := map[string]string{}
	nextCheckin := ""
	pendingCount := 0

	if a.orchestrator != nil {
		now := time.Now()
		var nextTime time.Time
		for key, runAt := range a.orchestrator.GetScheduledCheckinTimes() {
			plannedTimes[key] = runAt.Format("15:04")
			if runAt.After(now) {
				pendingCount++
				if nextTime.IsZero() || runAt.Before(nextTime) {
					nextTime = runAt
					label := key
					if entry, ok := cfg.Checkin[key]; ok && entry.Label != "" {
						label = entry.Label
					}
					nextCheckin = fmt.Sprintf("%s %s", label, runAt.Format("15:04"))
				}
			}
		}
	}

	return map[string]interface{}{
		"is_connected":  connected,
		"is_running":    running,
		"daily_results": results,
		"holiday":       hi,
		"checkin_times": times,
		"planned_times": plannedTimes,
		"next_checkin":  nextCheckin,
		"pending_count": pendingCount,
		"devices":       a.getDevices(),
		"config": map[string]interface{}{
			"host":    cfg.MuMu.Host,
			"port":    cfg.MuMu.Port,
			"package": cfg.App.PackageName,
		},
	}
}

// GetDashboard 返回仪表盘完整数据
func (a *App) GetDashboard() map[string]interface{} {
	return a.getDashboardData()
}

func (a *App) getDevices() []map[string]string {
	if a.adbHelper == nil {
		return nil
	}
	devices := []map[string]string{}
	for _, d := range a.adbHelper.Devices() {
		devices = append(devices, map[string]string{"serial": d.Serial, "status": d.Status})
	}
	return devices
}

// ConnectDevice 同步版本（兼容旧前端调用）—— 委托给异步版本
func (a *App) ConnectDevice() string {
	attempt := a.beginConnectAttempt()
	go a.connectDeviceSyncWithCallback(attempt)
	return "connecting"
}

func (a *App) connectDeviceSyncWithCallback(attempt uint64) {
	msg := a.connectDeviceSync(attempt)
	if !a.commitConnectAttempt(attempt) {
		slog.Info("忽略已取消的连接结果", "attempt", attempt, "message", msg)
		return
	}
	runtime.EventsEmit(a.ctx, "connect_result", map[string]interface{}{
		"success": a.isConnected,
		"message": msg,
	})
	time.Sleep(100 * time.Millisecond)
	a.emitStatus()
}

// postConnect 连接后的统一处理：打开APP + 登录检测 + 自动启动调度
func (a *App) postConnect(cfg *config.Config, wasLaunched bool) error {
	// 打开交建通APP
	pkg := cfg.App.PackageName
	slog.Info("正在打开应用", "package", pkg)
	opened := a.automator.OpenApp(pkg)
	if opened {
		slog.Info("应用已启动", "package", pkg)
		time.Sleep(3 * time.Second) // 等 APP 完全加载

		// 登录检测
		slog.Info("检查登录状态...")
		if err := a.automator.HandleLoginIfNeeded(60); err != nil {
			slog.Warn("登录检测未通过", "error", err)
			info := a.adbHelper.GetDeviceInfo("")
			if len(info) > 0 {
				runtime.EventsEmit(a.ctx, "device_info", info)
			}
			return err
		}
		slog.Info("应用已就绪")

		// 获取设备信息推送到前端
		info := a.adbHelper.GetDeviceInfo("")
		if len(info) > 0 {
			runtime.EventsEmit(a.ctx, "device_info", info)
		}
	} else {
		slog.Warn("应用启动失败", "package", pkg)
	}

	// 如果是从守护恢复过来的且之前期望运行，自动启动
	a.recovery.mu.Lock()
	wasRecover := a.recovery.inProgress
	a.recovery.mu.Unlock()

	if wasRecover && a.desiredRunning {
		slog.Info("守护恢复：重新启动调度")
		a.recovery.mu.Lock()
		a.recovery.inProgress = false
		a.recovery.mu.Unlock()
		a.StartScheduler()
		return nil
	}

	// 连接成功后，若开启了自动启动且非恢复流程，自动启动调度
	if !wasRecover && cfg.AppState.AutoStart {
		slog.Info("自动启动已开启，启动调度")
		a.StartScheduler()
	}
	return nil
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
	a.cancelConnectAttempt()
	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	if a.automator != nil {
		a.automator.Disconnect()
	}
	a.clearDisconnectedState()
	a.recovery.mu.Lock()
	a.recovery.inProgress = false
	a.recovery.nextRetryAt = time.Time{}
	a.recovery.pausedUntil = time.Time{}
	a.recovery.lastAction = "disconnect"
	a.recovery.lastResult = "idle"
	a.recovery.lastReason = ""
	a.recovery.lastError = ""
	a.recovery.mu.Unlock()
	a.emitGuardStatus()
	a.emitStatus()
	return "已断开"
}

// StartScheduler 启动打卡调度
func (a *App) StartScheduler() string {
	a.mu.Lock()
	if !a.isConnected {
		a.mu.Unlock()
		return "请先连接设备"
	}
	if a.isRunning {
		a.mu.Unlock()
		return "调度已在运行中"
	}
	a.mu.Unlock()

	if a.orchestrator == nil {
		a.orchestrator = automator.NewCheckinOrchestrator(
			a.automator, a.holidayChecker, a.configManager, a.baseDir,
		)
		if !a.orchestrator.Initialize() {
			return "调度器初始化失败"
		}

		// 注册结果回调 → 推送到前端实时显示
		a.orchestrator.SetResultCallback(func(r automator.CheckinRecord) {
			runtime.EventsEmit(a.ctx, "daily_result", map[string]interface{}{
				"job_id":      r.JobID,
				"action_name": r.ActionName,
				"success":     r.Success,
				"message":     r.Message,
				"timestamp":   r.Timestamp,
			})
			// 同时推送完整的仪表盘数据
			time.Sleep(100 * time.Millisecond)
			runtime.EventsEmit(a.ctx, "dashboard", a.getDashboardData())
		})
	}

	a.orchestrator.Start()
	a.mu.Lock()
	a.isRunning = true
	a.desiredRunning = true
	a.mu.Unlock()
	a.emitGuardStatus()
	a.emitStatus()

	// 推送仪表盘数据
	go func() {
		time.Sleep(500 * time.Millisecond)
		runtime.EventsEmit(a.ctx, "dashboard", a.getDashboardData())
	}()

	a.recovery.mu.Lock()
	recovering := a.recovery.inProgress
	a.recovery.mu.Unlock()
	if recovering {
		a.recovery.markSucceeded()
		a.emitGuardStatus()
	}

	return "调度已启动"
}

// StopScheduler 停止打卡调度
func (a *App) StopScheduler() string {
	if a.orchestrator != nil {
		a.orchestrator.Stop()
	}
	a.mu.Lock()
	a.isRunning = false
	a.desiredRunning = false
	a.pendingRecover = false
	a.mu.Unlock()
	a.recovery.mu.Lock()
	a.recovery.inProgress = false
	a.recovery.lastAction = "stop"
	a.recovery.lastResult = "idle"
	a.recovery.nextRetryAt = time.Time{}
	a.recovery.pausedUntil = time.Time{}
	a.recovery.mu.Unlock()
	a.emitGuardStatus()
	a.emitStatus()
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

	a.recordCheckinResult(checkinAction, result)
	return formatResult(result)
}

func (a *App) recordCheckinResult(action automator.CheckinAction, result *models.CheckinResult) {
	cfg := a.configManager.Config()
	utils.RecordCheckinResult(a.baseDir, result.Action, result.Success, result.Message, result.Timestamp)
	if cfg.Notification.Enabled {
		notifyCfg := utils.NotifyConfig{
			Enabled:   cfg.Notification.Enabled,
			Webhook:   cfg.Notification.Webhook,
			VerifyTLS: cfg.Notification.VerifyTLS,
		}
		utils.NotifyCheckinResult(notifyCfg, result.Action, result.Success, result.Message, result.Timestamp)
	}
}

func formatResult(result *models.CheckinResult) string {
	status := "成功"
	if !result.Success {
		status = "失败"
	}
	return fmt.Sprintf("打卡%s: %s [%s]", status, result.Message, result.Timestamp)
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
			"job_id":      r.JobID,
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

	oldCfg := a.configManager.Config()

	// 保留 app_state 的运行时状态
	if ok, msg := config.Validate(&newCfg); !ok {
		return "配置校验失败: " + msg
	}

	if oldCfg.AppState.BootAutoStart != newCfg.AppState.BootAutoStart {
		if err := autostart.SetEnabled(newCfg.AppState.BootAutoStart); err != nil {
			return "开机自启设置失败: " + err.Error()
		}
	}

	if err := a.configManager.SaveConfig(&newCfg); err != nil {
		return "配置保存失败: " + err.Error()
	}

	slog.Info("配置已保存并重载")
	a.refreshTray()
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

// GetDefaultConfig 返回默认配置 JSON
func (a *App) GetDefaultConfig() string {
	return config.DefaultConfigJSON()
}

// CheckHolidayUpdate 检查并更新节假日数据
func (a *App) CheckHolidayUpdate() string {
	if a.holidayChecker == nil {
		cfg := a.configManager.Config()
		a.holidayChecker = holiday.NewHolidayChecker(
			cfg.Holiday.SkipWeekend,
			cfg.Holiday.SkipHoliday,
			cfg.Holiday.ExtraWorkdays,
			cfg.Holiday.ExtraHolidays,
		)
		a.holidayChecker.SetLocalCachePath(filepath.Join(a.baseDir, "holidays.json"))
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

// CheckAppUpdate 检查软件更新（返回 JSON 格式的结果）
func (a *App) CheckAppUpdate() string {
	cfg := a.configManager.Config()
	url := cfg.Update.ManifestURL
	if url == "" {
		url = "https://raw.githubusercontent.com/juice4927/tongkatong-update/main/version.json"
	}
	asset, needsUpdate, err := updater.CheckUpdate(url, models.Version, "default")
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{
			"error": err.Error(),
		})
		return string(resp)
	}
	if !needsUpdate {
		resp, _ := json.Marshal(map[string]interface{}{
			"status":  "已是最新",
			"current": models.Version,
		})
		return string(resp)
	}
	resp, _ := json.Marshal(map[string]interface{}{
		"status":  "有新版本",
		"current": models.Version,
		"latest":  asset.Version,
		"notes":   asset.Notes,
		"size":    asset.Size,
	})
	return string(resp)
}

// ApplyUpdate 执行软件更新
func (a *App) ApplyUpdate() string {
	cfg := a.configManager.Config()
	url := cfg.Update.ManifestURL
	if url == "" {
		url = "https://raw.githubusercontent.com/juice4927/tongkatong-update/main/version.json"
	}
	asset, needsUpdate, err := updater.CheckUpdate(url, models.Version, "default")
	if err != nil {
		return "检查更新失败: " + err.Error()
	}
	if !needsUpdate {
		return "已是最新版本"
	}

	// 下载并启动更新器
	exe, _ := os.Executable()
	tmpDir := filepath.Join(os.TempDir(), updater.UpdateTempDir)
	_ = os.MkdirAll(tmpDir, 0755)
	destPath := filepath.Join(tmpDir, asset.FileName)

	a.emitUpdateStatus("下载更新中", fmt.Sprintf("正在下载 v%s", asset.Version), true, asset.Version)
	hash, err := updater.DownloadFile(asset.URL, destPath, func(downloaded, total int64) {
		a.emitUpdateProgress(downloaded, total)
	})
	if err != nil {
		a.emitUpdateStatus("下载失败", err.Error(), true, asset.Version)
		return "下载失败: " + err.Error()
	}
	if asset.SHA256 != "" && hash != asset.SHA256 {
		a.emitUpdateStatus("校验失败", "更新包 SHA256 校验失败", true, asset.Version)
		return "SHA256 校验失败"
	}

	updaterExe := filepath.Join(filepath.Dir(exe), "tongkatong-updater.exe")
	if err := updater.LaunchUpdater(updaterExe, destPath, exe, asset.Version, models.Version, os.Args[1:]); err != nil {
		a.emitUpdateStatus("启动更新器失败", err.Error(), true, asset.Version)
		return "启动更新器失败: " + err.Error()
	}
	a.emitUpdateStatus("更新包已就绪", fmt.Sprintf("v%s 已下载完成，准备重启更新", asset.Version), true, asset.Version)
	return "更新包已下载，程序即将更新重启"
}

// RefreshUpdateStatus 刷新更新状态
func (a *App) RefreshUpdateStatus() string {
	return updater.DescribeUpdateState(filepath.Dir(os.Args[0]))
}

// TestNotification 使用当前填写的参数发送一条测试通知
func (a *App) TestNotification(webhook string, verifyTLS bool) string {
	webhook = strings.TrimSpace(webhook)
	if webhook == "" {
		return "请先填写 Server酱 Key"
	}

	now := time.Now().Format("2006-01-02 15:04:05")
	title := "通卡通 测试通知"
	desp := fmt.Sprintf("这是一条测试通知。\n\n**时间**：%s\n\n如果你收到这条消息，说明通知配置可用。", now)
	if ok := utils.SendServerChan(webhook, title, desp, verifyTLS); !ok {
		return "测试通知发送失败，请检查 Key、网络或 TLS 设置"
	}
	return "测试通知已发送，请到微信中查看"
}

// ExportDiagnostics 导出诊断包，便于排查运行问题
func (a *App) ExportDiagnostics() string {
	defaultDir := filepath.Clean(filepath.Join(a.baseDir, ".."))
	defaultName := "tongkatong_diagnostics_" + time.Now().Format("20060102_150405") + ".zip"
	zipPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:            "导出诊断包",
		DefaultDirectory: defaultDir,
		DefaultFilename:  defaultName,
		Filters: []runtime.FileFilter{
			{DisplayName: "ZIP 压缩包 (*.zip)", Pattern: "*.zip"},
		},
	})
	if err != nil {
		return "导出诊断包失败: " + err.Error()
	}
	if zipPath == "" {
		return "已取消导出"
	}
	if !strings.HasSuffix(strings.ToLower(zipPath), ".zip") {
		zipPath += ".zip"
	}

	if err := a.writeDiagnosticsZip(zipPath); err != nil {
		return "导出诊断包失败: " + err.Error()
	}
	return "诊断包已导出: " + zipPath
}

func (a *App) writeDiagnosticsZip(zipPath string) error {
	file, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
	}()

	zipWriter := zip.NewWriter(file)
	defer zipWriter.Close()

	addBytes := func(name string, data []byte) error {
		w, err := zipWriter.Create(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	addFile := func(srcPath, name string) error {
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		return addBytes(name, data)
	}

	summary := map[string]interface{}{
		"version":      a.GetVersionInfo(),
		"status":       a.GetStatus(),
		"guard_status": a.GetGuardStatus(),
		"dashboard":    a.GetDashboard(),
		"exported_at":  time.Now().Format("2006-01-02 15:04:05"),
	}
	summaryData, _ := json.MarshalIndent(summary, "", "  ")
	if err := addBytes("summary/runtime_summary.json", summaryData); err != nil {
		return err
	}

	configDir := a.configManager.ConfigDir()
	entries := []struct {
		src string
		dst string
	}{
		{filepath.Join(configDir, "default.json"), "config/default.json"},
		{filepath.Join(configDir, "user_config.json"), "config/user_config.json"},
		{filepath.Join(filepath.Dir(os.Args[0]), updater.UpdateStateFile), "update/" + updater.UpdateStateFile},
	}
	for _, entry := range entries {
		if _, err := os.Stat(entry.src); err == nil {
			if err := addFile(entry.src, entry.dst); err != nil {
				return err
			}
		}
	}

	logDir := filepath.Join(a.baseDir, "..", "logs")
	logCandidates := []string{
		filepath.Join(logDir, "checkin_"+time.Now().Format("2006-01-02")+".log"),
		filepath.Join(a.baseDir, "logs", "checkin_records.log"),
	}
	for _, src := range logCandidates {
		if _, err := os.Stat(src); err == nil {
			if err := addFile(src, "logs/"+filepath.Base(src)); err != nil {
				return err
			}
		}
	}

	diagDir := filepath.Join(a.baseDir, "logs", "diagnosis")
	if files, err := os.ReadDir(diagDir); err == nil {
		sort.Slice(files, func(i, j int) bool {
			iInfo, iErr := files[i].Info()
			jInfo, jErr := files[j].Info()
			if iErr != nil || jErr != nil {
				return files[i].Name() > files[j].Name()
			}
			return iInfo.ModTime().After(jInfo.ModTime())
		})
		limit := 12
		if len(files) < limit {
			limit = len(files)
		}
		for _, entry := range files[:limit] {
			if entry.IsDir() {
				continue
			}
			src := filepath.Join(diagDir, entry.Name())
			if err := addFile(src, "logs/diagnosis/"+entry.Name()); err != nil {
				return err
			}
		}
	}

	if logContent := a.GetLogContent(); logContent != "日志文件不可用" {
		if err := addBytes("logs/current_log_snapshot.txt", []byte(logContent)); err != nil {
			return err
		}
	}

	if err := zipWriter.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	closed = true
	return nil
}
