package automator

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestCheckinScheduler_Initialize(t *testing.T) {
	s := NewCheckinScheduler()
	if !s.Initialize() {
		t.Error("Initialize should return true")
	}
}

func TestCheckinScheduler_InitializeTwice(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	// 第二次初始化应覆盖旧的 cron 实例
	s.Initialize()
	if !s.IsRunning() {
		s.Start()
	}
	if !s.IsRunning() {
		t.Error("scheduler should be running after Start")
	}
}

func TestCheckinScheduler_StartStop(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	if s.IsRunning() {
		t.Error("should not be running before Start")
	}

	s.Start()
	if !s.IsRunning() {
		t.Error("should be running after Start")
	}

	s.Stop()
	if s.IsRunning() {
		t.Error("should not be running after Stop")
	}
}

func TestCheckinScheduler_AddOnceJob(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	s.Start()
	defer s.Stop()

	var executed int32
	runAt := time.Now().Add(1200 * time.Millisecond)

	added := s.AddOnceJob("test_job", runAt, func() {
		atomic.AddInt32(&executed, 1)
	})
	if !added {
		t.Fatal("AddOnceJob should return true")
	}

	// 等待执行
	time.Sleep(2500 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 1 {
		t.Errorf("callback should have been executed once, got %d executions", executed)
	}
}

func TestCheckinScheduler_AddOnceJob_PastTime(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	runAt := time.Now().Add(-1 * time.Hour) // 在过去

	added := s.AddOnceJob("past_job", runAt, func() {})
	if added {
		t.Error("AddOnceJob with past time should return false")
	}
}

func TestCheckinScheduler_AddOnceJob_NotInitialized(t *testing.T) {
	s := NewCheckinScheduler()
	// 没有调用 Initialize

	added := s.AddOnceJob("job", time.Now().Add(1*time.Hour), func() {})
	if added {
		t.Error("AddOnceJob without Initialize should return false")
	}
}

func TestCheckinScheduler_ReplaceJob(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	s.Start()
	defer s.Stop()

	var firstExec, secondExec int32
	runAt := time.Now().Add(1200 * time.Millisecond)

	// 添加第一个任务
	s.AddOnceJob("replace_job", runAt, func() {
		atomic.AddInt32(&firstExec, 1)
	})

	// 立即替换为第二个任务（相同 jobID）
	s.AddOnceJob("replace_job", runAt, func() {
		atomic.AddInt32(&secondExec, 1)
	})

	time.Sleep(2500 * time.Millisecond)

	// 只有第二个任务应该被执行
	if atomic.LoadInt32(&firstExec) != 0 {
		t.Error("first callback should not have been executed (was replaced)")
	}
	if atomic.LoadInt32(&secondExec) != 1 {
		t.Errorf("second callback should have been executed once, got %d", atomic.LoadInt32(&secondExec))
	}
}

func TestCheckinScheduler_RemoveJob(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	s.Start()
	defer s.Stop()

	var executed int32
	runAt := time.Now().Add(1200 * time.Millisecond)

	s.AddOnceJob("removable_job", runAt, func() {
		atomic.AddInt32(&executed, 1)
	})

	// 在运行前移除
	s.RemoveJob("removable_job")

	time.Sleep(2500 * time.Millisecond)

	if atomic.LoadInt32(&executed) != 0 {
		t.Error("callback should not have been executed after RemoveJob")
	}

	// 确认已从 GetJobs 中移除
	jobs := s.GetJobs()
	if _, exists := jobs["removable_job"]; exists {
		t.Error("removed job should not appear in GetJobs")
	}
}

func TestCheckinScheduler_GetNextRunTime(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	runAt := time.Now().Add(1 * time.Hour)
	s.AddOnceJob("time_job", runAt, func() {})

	next := s.GetNextRunTime("time_job")
	if next == nil {
		t.Fatal("GetNextRunTime should not return nil for existing job")
	}
	if !next.Equal(runAt) {
		t.Errorf("GetNextRunTime = %v, want %v", next, runAt)
	}

	// 不存在的任务
	nonexistent := s.GetNextRunTime("nonexistent")
	if nonexistent != nil {
		t.Error("GetNextRunTime for nonexistent job should return nil")
	}
}

func TestCheckinScheduler_GetJobs(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	s.AddOnceJob("job1", time.Now().Add(1*time.Hour), func() {})
	s.AddOnceJob("job2", time.Now().Add(2*time.Hour), func() {})

	jobs := s.GetJobs()
	if len(jobs) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(jobs))
	}

	if _, ok := jobs["job1"]; !ok {
		t.Error("job1 missing from GetJobs")
	}
	if _, ok := jobs["job2"]; !ok {
		t.Error("job2 missing from GetJobs")
	}
}

func TestCheckinScheduler_MultipleJobs(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	s.Start()
	defer s.Stop()

	var count1, count2 int32
	base := time.Now().Add(1200 * time.Millisecond)

	s.AddOnceJob("multi1", base, func() {
		atomic.AddInt32(&count1, 1)
	})
	s.AddOnceJob("multi2", base.Add(500*time.Millisecond), func() {
		atomic.AddInt32(&count2, 1)
	})

	time.Sleep(3000 * time.Millisecond)

	if atomic.LoadInt32(&count1) != 1 {
		t.Errorf("multi1 should have executed once, got %d", atomic.LoadInt32(&count1))
	}
	if atomic.LoadInt32(&count2) != 1 {
		t.Errorf("multi2 should have executed once, got %d", atomic.LoadInt32(&count2))
	}
}

func TestCheckinScheduler_StopWhileRunningJobs(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()
	s.Start()

	var executed int32
	s.AddOnceJob("stop_job", time.Now().Add(1050*time.Millisecond), func() {
		atomic.AddInt32(&executed, 1)
	})

	// 快停止
	time.Sleep(30 * time.Millisecond)
	s.Stop()

	// 停止后不应 panic 或死锁
	time.Sleep(1500 * time.Millisecond)
	t.Logf("executed = %d (may be 0 or 1 depending on timing)", atomic.LoadInt32(&executed))

	// 停止后重新启动
	s.Initialize()
	s.Start()
	defer s.Stop()

	if !s.IsRunning() {
		t.Error("should be running after re-Initialize + Start")
	}
}

func TestCheckinScheduler_MultipleStarts(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	s.Start()
	s.Start() // 重复启动

	if !s.IsRunning() {
		t.Error("should be running after multiple Starts")
	}

	s.Stop()
}

func TestCheckinScheduler_RemoveNonExistentJob(t *testing.T) {
	s := NewCheckinScheduler()
	s.Initialize()

	// 移出不存在的 job 不应 panic
	s.RemoveJob("nonexistent")
}
