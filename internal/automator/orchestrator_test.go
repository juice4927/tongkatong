package automator

import (
	"errors"
	"testing"
	"time"

	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/models"
)

func TestBuildExecutionRecordPreservesClassifiedFailureWhenErrReturned(t *testing.T) {
	now := time.Date(2026, 6, 12, 8, 30, 0, 0, time.Local)
	result := &models.CheckinResult{
		Success:     false,
		Action:      string(MorningSignin),
		Message:     "导航到考勤页面失败",
		Timestamp:   "2026-06-12 08:29:59",
		FailureCode: string(models.NavigationFailed),
	}

	displayResult, record := buildExecutionRecord("morning_signin", "上午签到", result, errors.New("navigation failed"), now)

	if displayResult == nil {
		t.Fatal("displayResult should not be nil")
	}
	if displayResult.FailureCode != string(models.NavigationFailed) {
		t.Fatalf("failure code = %q, want %q", displayResult.FailureCode, models.NavigationFailed)
	}
	if displayResult.Message != "导航到考勤页面失败" {
		t.Fatalf("message = %q", displayResult.Message)
	}
	if displayResult.Action != "上午签到" {
		t.Fatalf("display action = %q, want label", displayResult.Action)
	}
	if record.JobID != "morning_signin" {
		t.Fatalf("record job id = %q", record.JobID)
	}
	if record.ActionName != "上午签到" {
		t.Fatalf("record action name = %q", record.ActionName)
	}
	if record.FailureCode != string(models.NavigationFailed) {
		t.Fatalf("record failure code = %q", record.FailureCode)
	}
}

func TestBuildExecutionRecordSynthesizesSystemErrorWhenResultMissing(t *testing.T) {
	now := time.Date(2026, 6, 12, 8, 30, 0, 0, time.Local)

	displayResult, record := buildExecutionRecord("morning_signin", "上午签到", nil, errors.New("boom"), now)

	if displayResult == nil {
		t.Fatal("displayResult should not be nil")
	}
	if displayResult.Success {
		t.Fatal("displayResult should be failed")
	}
	if displayResult.FailureCode != string(models.SystemError) {
		t.Fatalf("failure code = %q, want %q", displayResult.FailureCode, models.SystemError)
	}
	if displayResult.Message != "异常: boom" {
		t.Fatalf("message = %q", displayResult.Message)
	}
	if record.Timestamp != "2026-06-12 08:30:00" {
		t.Fatalf("timestamp = %q", record.Timestamp)
	}
}

func TestShouldSendDailySummaryRequiresUniqueEnabledSlots(t *testing.T) {
	checkins := map[string]config.CheckinEntry{
		"morning_signin":   {Enabled: true, Label: "上午签到"},
		"morning_signout":  {Enabled: true, Label: "上午签退"},
		"afternoon_signin": {Enabled: false, Label: "下午签到"},
	}
	results := []CheckinRecord{
		{JobID: "morning_signin", ActionName: "上午签到", Success: true},
		{JobID: "morning_signin", ActionName: "上午签到", Success: true},
	}

	if hasCompletedEnabledSlots(results, checkins) {
		t.Fatal("duplicate records for one slot must not complete the whole day")
	}

	results = append(results, CheckinRecord{JobID: "morning_signout", ActionName: "上午签退", Success: false})
	if !hasCompletedEnabledSlots(results, checkins) {
		t.Fatal("one result per enabled slot should complete the day, even when a slot failed")
	}
}

func TestShouldSendDailySummaryHonorsSentDateGuard(t *testing.T) {
	checkins := map[string]config.CheckinEntry{
		"morning_signin": {Enabled: true, Label: "上午签到"},
	}
	results := []CheckinRecord{
		{JobID: "morning_signin", ActionName: "上午签到", Success: true},
	}

	if !shouldSendDailySummary(results, checkins, "", "2026-06-12") {
		t.Fatal("completed day with no sent marker should send")
	}
	if shouldSendDailySummary(results, checkins, "2026-06-12", "2026-06-12") {
		t.Fatal("summary should not send twice on the same date")
	}
}
