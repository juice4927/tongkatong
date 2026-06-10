package models

import (
	"testing"
)

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code    FailureCode
		retryable bool
	}{
		{DeviceConnectFailed, true},
		{DeviceUnresponsive, true},
		{DeviceConnectionFailed, true},
		{NavigationFailed, true},
		{ButtonNotFound, true},
		{NetworkError, true},
		{AppPopupFailed, true},
		{CheckinFailed, true},
		{GpsRuntimeFailed, true},

		{DeviceNotConnected, false},
		{GpsPreCheckFailed, false},
		{LoginTimeout, false},
		{AppNotFound, false},
		{AlreadyCheckedIn, false},
		{OutsideRange, false},
		{SchedulerError, false},
		{SystemError, false},
		{ResultUnconfirmed, false},
	}

	for _, tc := range tests {
		t.Run(string(tc.code), func(t *testing.T) {
			got := IsRetryable(tc.code)
			if got != tc.retryable {
				t.Errorf("IsRetryable(%q) = %v, want %v", tc.code, got, tc.retryable)
			}
		})
	}
}

func TestIsRetryable_UnknownCode(t *testing.T) {
	unknown := FailureCode("unknown_code")
	if IsRetryable(unknown) {
		t.Error("unknown code should not be retryable")
	}
}

func TestIsRetryable_EmptyCode(t *testing.T) {
	if IsRetryable("") {
		t.Error("empty code should not be retryable")
	}
}

// TestAllFailureCodesAreValid 确保所有常量不是空字符串
func TestAllFailureCodesAreValid(t *testing.T) {
	codes := []FailureCode{
		DeviceConnectFailed, DeviceNotConnected, DeviceUnresponsive, DeviceConnectionFailed,
		NavigationFailed, ButtonNotFound, ResultUnconfirmed,
		GpsPreCheckFailed, GpsRuntimeFailed,
		LoginTimeout, AppNotFound, AppPopupFailed,
		AlreadyCheckedIn, OutsideRange, NetworkError, CheckinFailed,
		SchedulerError, SystemError,
	}

	for _, code := range codes {
		if code == "" {
			t.Error("a FailureCode constant is empty")
		}
	}
}

func TestCheckinResultFields(t *testing.T) {
	result := CheckinResult{
		Success:        true,
		Action:         "morning_signin",
		Message:        "打卡成功",
		Timestamp:      "2025-01-06 08:30:00",
		ScreenshotPath: "/tmp/screenshot.png",
		FailureCode:    "",
		RecoveryAction: "",
	}

	if !result.Success {
		t.Error("Success should be true")
	}
	if result.Action != "morning_signin" {
		t.Errorf("Action = %s, want morning_signin", result.Action)
	}
}
