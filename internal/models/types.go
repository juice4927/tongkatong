// Package models 定义项目中共享的数据类型。
package models

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
