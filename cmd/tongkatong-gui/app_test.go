package main

import (
	"strings"
	"testing"

	"github.com/juice4927/tongkatong/internal/config"
)

func TestConnectAttemptStaleAfterCancel(t *testing.T) {
	app := &App{}

	attempt := app.beginConnectAttempt()
	if !app.isConnectAttemptActive(attempt) {
		t.Fatal("new connect attempt should be active")
	}

	app.cancelConnectAttempt()

	if app.isConnectAttemptActive(attempt) {
		t.Fatal("cancelled connect attempt should be stale")
	}
	if app.commitConnectAttempt(attempt) {
		t.Fatal("stale connect attempt must not be committed")
	}
}

func TestOnlyLatestConnectAttemptCanCommit(t *testing.T) {
	app := &App{}

	first := app.beginConnectAttempt()
	second := app.beginConnectAttempt()

	if app.commitConnectAttempt(first) {
		t.Fatal("older connect attempt must not be committed")
	}
	if !app.commitConnectAttempt(second) {
		t.Fatal("latest active connect attempt should commit")
	}
	if app.connecting {
		t.Fatal("committed connect attempt should clear connecting state")
	}
}

func TestDisconnectStateClearsSchedulerState(t *testing.T) {
	app := &App{
		isConnected:    true,
		isRunning:      true,
		desiredRunning: true,
		pendingRecover: true,
	}

	app.clearDisconnectedState()

	if app.isConnected {
		t.Fatal("disconnect should clear connected state")
	}
	if app.isRunning {
		t.Fatal("disconnect should clear running state")
	}
	if app.desiredRunning {
		t.Fatal("disconnect should clear desired running state")
	}
	if app.pendingRecover {
		t.Fatal("disconnect should clear pending recovery state")
	}
}

func TestCheckHolidayUpdateDoesNotRequireDeviceConnection(t *testing.T) {
	app := &App{
		configManager: config.NewConfigManager(t.TempDir()),
		baseDir:       t.TempDir(),
	}

	msg := app.CheckHolidayUpdate()

	if strings.Contains(msg, "请先连接设备") {
		t.Fatalf("holiday update should not require device connection, got %q", msg)
	}
}
