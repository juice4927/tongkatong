package automator

import (
	"log/slog"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
)

// ── CheckinScheduler ────────────────────────────────────────────────

// CheckinScheduler 定时调度器
type CheckinScheduler struct {
	mu        sync.Mutex
	cron      *cron.Cron
	jobs      map[string]*ScheduledJob
	entries   map[string]cron.EntryID // jobID → cron.EntryID
	running   bool
}

// ScheduledJob 定时任务信息
type ScheduledJob struct {
	ID       string
	RunAt    time.Time
	Status   string // pending / running / completed / failed
	Callback func()
}

// NewCheckinScheduler 创建调度器
func NewCheckinScheduler() *CheckinScheduler {
	return &CheckinScheduler{
		jobs:    make(map[string]*ScheduledJob),
		entries: make(map[string]cron.EntryID),
	}
}

// Initialize 初始化调度器
func (s *CheckinScheduler) Initialize() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cron = cron.New(
		cron.WithSeconds(),
		cron.WithChain(
			cron.SkipIfStillRunning(cron.VerbosePrintfLogger(slog.NewLogLogger(slog.Default().Handler(), slog.LevelDebug))),
		),
	)
	slog.Info("调度器初始化完成")
	return true
}

// Start 启动调度器
func (s *CheckinScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil && !s.running {
		s.cron.Start()
		s.running = true
		slog.Info("调度器已启动")
	}
}

// Stop 停止调度器
func (s *CheckinScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil && s.running {
		ctx := s.cron.Stop()
		<-ctx.Done()
		s.running = false
		s.jobs = make(map[string]*ScheduledJob)
		s.entries = make(map[string]cron.EntryID)
		slog.Info("调度器已停止")
	}
}

// AddOnceJob 添加一次性定时任务
func (s *CheckinScheduler) AddOnceJob(jobID string, runAt time.Time, callback func()) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cron == nil {
		slog.Error("调度器未初始化")
		return false
	}

	// 移除旧的 cron entry 和本地缓存
	s.removeJobLocked(jobID)

	delay := time.Until(runAt)
	if delay < 0 {
		slog.Warn("任务时间已过，跳过", "job", jobID, "runAt", runAt)
		return false
	}

	cronExpr := runAt.Format("05 04 15 02 01 *")
	entryID, err := s.cron.AddFunc(cronExpr, callback)
	if err != nil {
		slog.Error("添加任务失败", "job", jobID, "error", err)
		return false
	}

	s.jobs[jobID] = &ScheduledJob{
		ID:       jobID,
		RunAt:    runAt,
		Status:   "pending",
		Callback: callback,
	}
	s.entries[jobID] = entryID

	slog.Info("添加定时任务", "job", jobID, "runAt", runAt.Format("15:04:05"), "delay", delay.Round(time.Second))
	return true
}

// RemoveJob 移除任务
func (s *CheckinScheduler) RemoveJob(jobID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.removeJobLocked(jobID)
}

func (s *CheckinScheduler) removeJobLocked(jobID string) {
	// 从 cron 调度器中移除
	if entryID, ok := s.entries[jobID]; ok && s.cron != nil {
		s.cron.Remove(entryID)
		delete(s.entries, jobID)
	}
	// 从本地缓存中移除
	delete(s.jobs, jobID)
}

// GetJobs 获取所有任务
func (s *CheckinScheduler) GetJobs() map[string]*ScheduledJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string]*ScheduledJob)
	for k, v := range s.jobs {
		result[k] = v
	}
	return result
}

// GetNextRunTime 获取任务下次运行时间
func (s *CheckinScheduler) GetNextRunTime(jobID string) *time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, exists := s.jobs[jobID]
	if !exists {
		return nil
	}
	return &job.RunAt
}

// IsRunning 检查调度器是否在运行
func (s *CheckinScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}
