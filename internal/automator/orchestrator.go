package automator

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/holiday"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/utils"
)

// ── CheckinOrchestrator ────────────────────────────────────────────

// CheckinOrchestrator 打卡协调器
type CheckinOrchestrator struct {
	mu             sync.Mutex
	automator      *UIAutomator2Impl
	holidayChecker *holiday.HolidayChecker
	configManager  *config.ConfigManager
	scheduler      *CheckinScheduler
	baseDir        string

	checkinTimes    map[string]time.Time
	dailyResults    []CheckinRecord
	scheduledDate   time.Time
	summarySentDate string
	runningJobIDs   map[string]bool
	rescheduleTimer *time.Timer

	lastResult     *models.CheckinResult
	lastResultMeta string

	// 事件回调（供 GUI 层注册）
	onResult func(result CheckinRecord)
	onError  func(jobID string, err error)
}

// CheckinRecord 打卡记录
type CheckinRecord struct {
	JobID       string
	ActionName  string
	Success     bool
	Message     string
	Timestamp   string
	FailureCode string
}

// NewCheckinOrchestrator 创建打卡协调器
func NewCheckinOrchestrator(automator *UIAutomator2Impl, hc *holiday.HolidayChecker, cm *config.ConfigManager, baseDir string) *CheckinOrchestrator {
	automator.SetBaseDir(baseDir)
	return &CheckinOrchestrator{
		automator:      automator,
		holidayChecker: hc,
		configManager:  cm,
		scheduler:      NewCheckinScheduler(),
		baseDir:        baseDir,
		checkinTimes:   make(map[string]time.Time),
		dailyResults:   make([]CheckinRecord, 0),
		runningJobIDs:  make(map[string]bool),
	}
}

// SetResultCallback 设置结果回调（每次打卡完成时调用）
func (co *CheckinOrchestrator) SetResultCallback(cb func(CheckinRecord)) {
	co.onResult = cb
}

// Initialize 初始化
func (co *CheckinOrchestrator) Initialize() bool {
	if !co.scheduler.Initialize() {
		return false
	}
	return true
}

// Start 启动调度
func (co *CheckinOrchestrator) Start() {
	co.mu.Lock()
	co.scheduler.Start()
	co.mu.Unlock()
	co.scheduleToday("startup")
	co.setupDailyReschedule()
}

// Stop 停止
func (co *CheckinOrchestrator) Stop() {
	co.mu.Lock()
	defer co.mu.Unlock()

	// 取消每日重调度定时器
	if co.rescheduleTimer != nil {
		co.rescheduleTimer.Stop()
		co.rescheduleTimer = nil
	}

	co.scheduler.Stop()
	co.checkinTimes = make(map[string]time.Time)
}

// GetDailyResults 获取当天打卡结果
func (co *CheckinOrchestrator) GetDailyResults() []CheckinRecord {
	co.mu.Lock()
	defer co.mu.Unlock()
	result := make([]CheckinRecord, len(co.dailyResults))
	copy(result, co.dailyResults)
	return result
}

// GetScheduledCheckinTimes 返回今天已生成的随机打卡时间
func (co *CheckinOrchestrator) GetScheduledCheckinTimes() map[string]time.Time {
	co.mu.Lock()
	defer co.mu.Unlock()
	result := make(map[string]time.Time, len(co.checkinTimes))
	for key, value := range co.checkinTimes {
		result[key] = value
	}
	return result
}

// ── 调度 ───────────────────────────────────────────────────────────

func (co *CheckinOrchestrator) scheduleToday(source string) {
	cfg := co.configManager.Config()
	checkinConfigs := cfg.Checkin

	// 检查今天是否工作日
	today := time.Now()
	if !co.holidayChecker.IsWorkday(today) {
		slog.Info("今天不是工作日，跳过打卡")
		co.mu.Lock()
		co.scheduledDate = today
		co.checkinTimes = make(map[string]time.Time)
		co.mu.Unlock()
		return
	}

	// 生成打卡时间
	delayMin := cfg.RandomDelay.MinSeconds
	delayMax := cfg.RandomDelay.MaxSeconds

	times := make(map[string]time.Time)
	for key, ct := range checkinConfigs {
		if !ct.Enabled || len(ct.TimeRange) != 2 {
			continue
		}
		// 只有配置了有效时间范围才生成
		if ct.TimeRange[0] == "00:00" && ct.TimeRange[1] == "00:00" {
			continue
		}
		times[key] = GenerateRandomTime(ct.TimeRange[0], ct.TimeRange[1], delayMin, delayMax)
	}

	co.mu.Lock()
	co.checkinTimes = times
	co.scheduledDate = today
	co.summarySentDate = ""
	co.dailyResults = make([]CheckinRecord, 0)
	co.mu.Unlock()

	// 添加定时任务
	now := time.Now()
	scheduled := 0
	makeup := 0

	for jobID, runAt := range times {
		action, ok := actionMap[jobID]
		if !ok {
			continue
		}

		if runAt.After(now) {
			// 未来时间 → 定时执行
			label := checkinConfigs[jobID].Label
			actionCopy := action
			labelCopy := label
			jobIDCopy := jobID

			co.scheduler.AddOnceJob(jobID, runAt, func() {
				co.executeCheckin(actionCopy, jobIDCopy, labelCopy)
			})
			scheduled++
		} else {
			// 已过时间 → 检查补签窗口
			if co.isInMakeupWindow(jobID, now) {
				makeup++
				label := checkinConfigs[jobID].Label
				actionCopy := action
				labelCopy := label
				jobIDCopy := jobID

				// 延时执行补签
				co.scheduler.AddOnceJob(jobID, now.Add(2*time.Second), func() {
					co.executeCheckin(actionCopy, jobIDCopy, labelCopy)
				})
			}
		}
	}

	slog.Info("调度摘要",
		"source", source,
		"scheduled", scheduled,
		"makeup", makeup,
	)
}

func (co *CheckinOrchestrator) setupDailyReschedule() {
	now := time.Now()
	nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 1, 0, 0, now.Location())
	delay := time.Until(nextMidnight)

	co.mu.Lock()
	if co.rescheduleTimer != nil {
		co.rescheduleTimer.Stop()
	}
	co.rescheduleTimer = time.AfterFunc(delay, func() {
		co.mu.Lock()
		isRunning := co.scheduler.IsRunning()
		co.mu.Unlock()
		if !isRunning {
			return // 已停止，不再重调度
		}
		slog.Info("跨日重调度")
		co.scheduleToday("crossday")
		co.setupDailyReschedule()
	})
	co.mu.Unlock()
}

func (co *CheckinOrchestrator) isInMakeupWindow(jobID string, now time.Time) bool {
	cfg := co.configManager.Config()
	var window []int

	switch jobID {
	case "morning_signin":
		window = cfg.MakeupWindow.MorningSignin
	case "morning_signout":
		window = cfg.MakeupWindow.MorningSignout
	case "afternoon_signin":
		window = cfg.MakeupWindow.AfternoonSignin
	case "afternoon_signout":
		window = cfg.MakeupWindow.AfternoonSignout
	}

	if len(window) != 4 {
		return false
	}

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := window[0]*60 + window[1]
	endMinutes := window[2]*60 + window[3]

	if endMinutes > 1440 {
		// 跨天窗口
		return nowMinutes >= startMinutes || nowMinutes < (endMinutes-1440)
	}
	return nowMinutes >= startMinutes && nowMinutes < endMinutes
}

// ── 结果查询 ────────────────────────────────────────────────────

// GetLastResult 获取最近一次打卡结果
func (co *CheckinOrchestrator) GetLastResult() *models.CheckinResult {
	co.mu.Lock()
	defer co.mu.Unlock()
	return co.lastResult
}

// ShouldRetryCode 判断失败码是否应该触发自动重试
func ShouldRetryCode(fc string) bool {
	switch models.FailureCode(fc) {
	case models.DeviceConnectFailed, models.DeviceUnresponsive, models.DeviceConnectionFailed,
		models.NavigationFailed, models.ButtonNotFound, models.NetworkError, models.AppPopupFailed,
		models.CheckinFailed, models.GpsRuntimeFailed:
		return true
	default:
		return false
	}
}

// ── 执行打卡 ───────────────────────────────────────────────────────

func (co *CheckinOrchestrator) executeCheckin(action CheckinAction, jobID, label string) {
	// 防重入
	co.mu.Lock()
	if co.runningJobIDs[jobID] {
		co.mu.Unlock()
		slog.Warn("任务已在执行中，跳过", "job", jobID)
		return
	}
	co.runningJobIDs[jobID] = true
	co.mu.Unlock()

	defer func() {
		co.mu.Lock()
		delete(co.runningJobIDs, jobID)
		co.mu.Unlock()
	}()

	// 网络检查
	if !utils.CheckNetworkConnectivity(nil, 5*time.Second) {
		slog.Warn("网络不可用，等待 30 秒后重试")
		time.Sleep(30 * time.Second)
		if !utils.CheckNetworkConnectivity(nil, 5*time.Second) {
			slog.Error("网络仍不可用，跳过本次打卡")
			co.recordRecord(CheckinRecord{
				JobID:       jobID,
				ActionName:  label,
				Success:     false,
				Message:     "网络不可用",
				Timestamp:   time.Now().Format("2006-01-02 15:04:05"),
				FailureCode: string(models.NetworkError),
			})
			return
		}
	}

	// 再次检查是否工作日
	if !co.holidayChecker.IsWorkday(time.Now()) {
		slog.Info("今天不是工作日，跳过", "label", label)
		return
	}

	// 设置 GPS（两个坐标都非零时才设置）
	cfg := co.configManager.Config()
	if cfg.MuMu.GpsLatitude != 0 && cfg.MuMu.GpsLongitude != 0 {
		if err := co.automator.SetGPS(cfg.MuMu.GpsLatitude, cfg.MuMu.GpsLongitude); err != nil {
			slog.Warn("GPS 设置失败，继续尝试打卡", "error", err)
		} else {
			// GPS 预检：设置后等待 2 秒，检测定位是否生效
			time.Sleep(2 * time.Second)
			xml, _ := co.automator.DumpHierarchy()
			if strings.Contains(xml, "定位失败") || strings.Contains(xml, "请先定位") {
				slog.Warn("GPS 预检查到定位异常，继续尝试")
			}
		}
	}

	// 执行打卡
	slog.Info("开始执行打卡", "label", label)
	result, err := co.automator.DoCheckin(action)
	if err != nil {
		slog.Error("打卡异常", "label", label, "error", err)
	}
	displayResult, record := buildExecutionRecord(jobID, label, result, err, time.Now())

	co.mu.Lock()
	co.lastResult = displayResult
	if displayResult.Success {
		co.lastResultMeta = "success"
	} else {
		co.lastResultMeta = displayResult.FailureCode
	}
	co.mu.Unlock()

	co.recordRecord(record)

	// 打卡结果通知
	co.notifyResult(displayResult)

	// 触发 GUI 回调
	if co.onResult != nil {
		go co.onResult(record)
	}

	// 检查是否需要发送每日汇总（当天最后一次打卡完成后）
	co.maybeSendDailySummary()
}

func (co *CheckinOrchestrator) recordResult(actionName string, success bool, message, timestamp, failureCode string) {
	co.recordRecord(CheckinRecord{
		ActionName:  actionName,
		Success:     success,
		Message:     message,
		Timestamp:   timestamp,
		FailureCode: failureCode,
	})
}

func (co *CheckinOrchestrator) recordRecord(record CheckinRecord) {
	co.mu.Lock()
	co.dailyResults = append(co.dailyResults, record)
	co.mu.Unlock()

	// 写入记录文件
	utils.RecordCheckinResult(co.baseDir, record.ActionName, record.Success, record.Message, record.Timestamp)
}

func (co *CheckinOrchestrator) notifyResult(result *models.CheckinResult) {
	cfg := co.configManager.Config()
	notifyCfg := utils.NotifyConfig{
		Enabled:   cfg.Notification.Enabled,
		Webhook:   cfg.Notification.Webhook,
		VerifyTLS: cfg.Notification.VerifyTLS,
	}
	utils.NotifyCheckinResult(notifyCfg, result.Action, result.Success, result.Message, result.Timestamp)
}

// maybeSendDailySummary 今天最后一次打卡完成后发送每日汇总
func (co *CheckinOrchestrator) maybeSendDailySummary() {
	co.mu.Lock()
	results := make([]CheckinRecord, len(co.dailyResults))
	copy(results, co.dailyResults)
	lastSent := co.summarySentDate
	co.mu.Unlock()

	cfg := co.configManager.Config()
	if !cfg.Notification.Enabled || cfg.Notification.Webhook == "" {
		return
	}

	today := time.Now().Format("2006-01-02")
	if shouldSendDailySummary(results, cfg.Checkin, lastSent, today) {
		co.mu.Lock()
		if co.summarySentDate == today {
			co.mu.Unlock()
			return
		}
		co.summarySentDate = today
		co.mu.Unlock()
		co.sendDailySummary(results, cfg)
	}
}

func buildExecutionRecord(jobID, label string, result *models.CheckinResult, err error, now time.Time) (*models.CheckinResult, CheckinRecord) {
	displayAction := label
	if displayAction == "" {
		displayAction = jobID
	}

	if result == nil {
		message := "异常"
		if err != nil {
			message = "异常: " + err.Error()
		}
		result = &models.CheckinResult{
			Success:     false,
			Action:      string(actionMap[jobID]),
			Message:     message,
			Timestamp:   now.Format("2006-01-02 15:04:05"),
			FailureCode: string(models.SystemError),
		}
	}
	if result.Timestamp == "" {
		result.Timestamp = now.Format("2006-01-02 15:04:05")
	}
	if result.FailureCode == "" && !result.Success {
		result.FailureCode = string(models.CheckinFailed)
	}

	displayResult := *result
	displayResult.Action = displayAction
	record := CheckinRecord{
		JobID:       jobID,
		ActionName:  displayAction,
		Success:     displayResult.Success,
		Message:     displayResult.Message,
		Timestamp:   displayResult.Timestamp,
		FailureCode: displayResult.FailureCode,
	}
	return &displayResult, record
}

func shouldSendDailySummary(results []CheckinRecord, checkins map[string]config.CheckinEntry, sentDate, today string) bool {
	if sentDate == today {
		return false
	}
	return hasCompletedEnabledSlots(results, checkins)
}

func hasCompletedEnabledSlots(results []CheckinRecord, checkins map[string]config.CheckinEntry) bool {
	enabled := make(map[string]struct{})
	labels := make(map[string]string)
	for jobID, entry := range checkins {
		if !entry.Enabled {
			continue
		}
		enabled[jobID] = struct{}{}
		if entry.Label != "" {
			labels[entry.Label] = jobID
		}
	}
	if len(enabled) == 0 {
		return false
	}

	completed := make(map[string]struct{})
	for _, result := range results {
		jobID := result.JobID
		if jobID == "" {
			jobID = labels[result.ActionName]
		}
		if _, ok := enabled[jobID]; ok {
			completed[jobID] = struct{}{}
		}
	}
	return len(completed) >= len(enabled)
}

func (co *CheckinOrchestrator) sendDailySummary(results []CheckinRecord, cfg *config.Config) {
	today := time.Now().Format("2006-01-02")
	title := fmt.Sprintf("通卡通 %s 打卡汇总", today)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**日期**：%s\n\n", today))
	sb.WriteString("**打卡结果**：\n")
	for _, r := range results {
		status := "✅"
		if !r.Success {
			status = "❌"
		}
		sb.WriteString(fmt.Sprintf("- %s %s: %s (%s)\n", status, r.ActionName, r.Message, r.Timestamp))
	}

	utils.SendServerChan(cfg.Notification.Webhook, title, sb.String(), cfg.Notification.VerifyTLS)
}

// RescheduleToday 手动触发今日重调度
func (co *CheckinOrchestrator) RescheduleToday() {
	co.scheduleToday("manual")
}

// actionMap 打卡动作映射
var actionMap = map[string]CheckinAction{
	"morning_signin":    MorningSignin,
	"morning_signout":   MorningSignout,
	"afternoon_signin":  AfternoonSignin,
	"afternoon_signout": AfternoonSignout,
}
