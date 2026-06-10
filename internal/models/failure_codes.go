// Package models 提供失败码枚举
package models

// FailureCode 打卡失败码（仅定义分类，不含展示逻辑）
type FailureCode string

const (
	// 设备连接类
	DeviceConnectFailed    FailureCode = "device_connect_failed"
	DeviceNotConnected     FailureCode = "device_not_connected"
	DeviceUnresponsive     FailureCode = "device_unresponsive"
	DeviceConnectionFailed FailureCode = "device_connection_failed"

	// 导航/界面类
	NavigationFailed    FailureCode = "navigation_failed"
	ButtonNotFound      FailureCode = "button_not_found"
	ResultUnconfirmed   FailureCode = "result_unconfirmed"

	// GPS 类
	GpsPreCheckFailed  FailureCode = "gps_precheck_failed"
	GpsRuntimeFailed   FailureCode = "gps_runtime_failed"

	// 应用/登录类
	LoginTimeout       FailureCode = "login_timeout"
	AppNotFound        FailureCode = "app_not_found"
	AppPopupFailed     FailureCode = "app_popup_failed"

	// 打卡逻辑类
	AlreadyCheckedIn   FailureCode = "already_checked_in"
	OutsideRange       FailureCode = "outside_range"
	NetworkError       FailureCode = "network_error"
	CheckinFailed      FailureCode = "checkin_failed"

	// 调度/系统类
	SchedulerError     FailureCode = "scheduler_error"
	SystemError        FailureCode = "system_error"
)

// IsRetryable 判断是否应自动重试
func IsRetryable(fc FailureCode) bool {
	switch fc {
	case DeviceConnectFailed, DeviceUnresponsive, DeviceConnectionFailed,
		NavigationFailed, ButtonNotFound, NetworkError, AppPopupFailed,
		CheckinFailed, GpsRuntimeFailed:
		return true
	default:
		return false
	}
}
