package utils

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// RecordCheckinResult 写入打卡记录到独立日志文件
func RecordCheckinResult(baseDir string, actionName string, success bool, message string, timestamp string) {
	if baseDir == "" {
		baseDir = "."
	}
	logDir := filepath.Join(baseDir, "logs")
	_ = os.MkdirAll(logDir, 0755)

	recordFile := filepath.Join(logDir, "checkin_records.log")
	status := "成功"
	if !success {
		status = "失败"
	}
	line := fmt.Sprintf("%s | %s | %s | %s\n", timestamp, actionName, status, message)

	f, err := os.OpenFile(recordFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Warn("写入打卡记录失败", "error", err)
		return
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		slog.Warn("写入打卡记录失败", "error", err)
	}
}

// SaveFailureDiagnosis 保存失败诊断信息（截图+XML 到 logs/diagnosis/）
func SaveFailureDiagnosis(baseDir string, actionName string, screenshotFn func() ([]byte, error), xmlDumpFn func() (string, error)) {
	if baseDir == "" {
		baseDir = "."
	}
	diagDir := filepath.Join(baseDir, "logs", "diagnosis")
	_ = os.MkdirAll(diagDir, 0755)

	now := time.Now()
	ts := now.Format("20060102_150405")
	prefix := fmt.Sprintf("fail_%s_%s", actionName, ts)

	// 保存截图
	if screenshotFn != nil {
		if data, err := screenshotFn(); err == nil && len(data) > 0 {
			path := filepath.Join(diagDir, prefix+".png")
			if writeErr := os.WriteFile(path, data, 0644); writeErr == nil {
				slog.Info("失败诊断截图已保存", "path", path)
			}
		}
	}

	// 保存 XML dump
	if xmlDumpFn != nil {
		if xml, err := xmlDumpFn(); err == nil && xml != "" {
			path := filepath.Join(diagDir, prefix+".xml")
			if writeErr := os.WriteFile(path, []byte(xml), 0644); writeErr == nil {
				slog.Info("失败诊断 XML 已保存", "path", path)
			}
		}
	}
}
