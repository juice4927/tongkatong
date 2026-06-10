// Package models 定义项目中共享的数据类型。
package models

import "time"

// CheckinAction 打卡动作类型
type CheckinAction string

const (
	MorningSignin    CheckinAction = "morning_signin"
	MorningSignout   CheckinAction = "morning_signout"
	AfternoonSignin  CheckinAction = "afternoon_signin"
	AfternoonSignout CheckinAction = "afternoon_signout"
)

// AllActions 返回所有打卡动作
func AllActions() []CheckinAction {
	return []CheckinAction{MorningSignin, MorningSignout, AfternoonSignin, AfternoonSignout}
}

// CheckinResult 打卡结果
type CheckinResult struct {
	Success        bool
	Action         string
	Message        string
	Timestamp      string
	ScreenshotPath string
	FailureCode    string
	RecoveryAction string
}

// DeviceSession 设备会话（含 TTL）
type DeviceSession struct {
	Address   string
	CreatedAt time.Time
	TTL       time.Duration
}

func (s *DeviceSession) IsExpired() bool {
	return time.Since(s.CreatedAt) > s.TTL
}

// JobStatus 定时任务状态
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobSkipped   JobStatus = "skipped"
)

// JobInfo 定时任务信息
type JobInfo struct {
	Time     time.Time
	Status   JobStatus
	Action   CheckinAction
	Label    string
	Callback func()
}
