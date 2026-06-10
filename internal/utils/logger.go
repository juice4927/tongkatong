// Package utils 提供日志管理功能
package utils

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// LogLevel 日志级别
type LogLevel string

const (
	LevelDebug LogLevel = "DEBUG"
	LevelInfo  LogLevel = "INFO"
	LevelWarn  LogLevel = "WARN"
	LevelError LogLevel = "ERROR"
)

// LogCallback 日志回调（供 GUI 桥接使用）
type LogCallback func(message string, level string)

// LogManager 日志管理器
type LogManager struct {
	mu        sync.RWMutex
	callbacks []LogCallback
	logger    *slog.Logger
	logDir    string
	logFile   *os.File
}

var (
	globalManager *LogManager
	globalMu      sync.Mutex
)

// GetLogManager 返回全局 LogManager 实例
func GetLogManager() *LogManager {
	globalMu.Lock()
	defer globalMu.Unlock()
	if globalManager == nil {
		globalManager = &LogManager{}
	}
	return globalManager
}

// SetupLogging 初始化日志系统
//
//	logDir: 日志目录（空=仅控制台）
//	level: 日志级别 DEBUG/INFO
//	consoleOutput: 是否输出到控制台
func SetupLogging(logDir string, level string, consoleOutput bool) error {
	lm := GetLogManager()
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.logDir = logDir

	var logLevel slog.Level
	switch strings.ToUpper(level) {
	case "DEBUG":
		logLevel = slog.LevelDebug
	default:
		logLevel = slog.LevelInfo
	}

	var writers []io.Writer

	if consoleOutput {
		writers = append(writers, os.Stdout)
	}

	if logDir != "" {
		_ = os.MkdirAll(logDir, 0755)
		now := time.Now()
		logFilePath := filepath.Join(logDir, fmt.Sprintf("checkin_%s.log", now.Format("2006-01-02")))
		f, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			lm.logFile = f
			writers = append(writers, f)
		}
	}

	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else if len(writers) > 1 {
		writer = io.MultiWriter(writers...)
	} else {
		writer = io.Discard
	}

	handler := slog.NewTextHandler(writer, &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05"))
			}
			return a
		},
	})

	lm.logger = slog.New(handler)
	slog.SetDefault(lm.logger)

	slog.Info("日志系统初始化完成", "logDir", logDir, "level", level)
	return nil
}

// AddCallback 注册日志回调
func (lm *LogManager) AddCallback(cb LogCallback) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.callbacks = append(lm.callbacks, cb)
}

// RemoveCallback 移除日志回调
func (lm *LogManager) RemoveCallback(cb LogCallback) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	for i, c := range lm.callbacks {
		if fmt.Sprintf("%p", c) == fmt.Sprintf("%p", cb) {
			lm.callbacks = append(lm.callbacks[:i], lm.callbacks[i+1:]...)
			return
		}
	}
}

// notifyCallbacks 通知所有回调（内部方法，调用方需持有锁）
func (lm *LogManager) notifyCallbacks(msg string, level string) {
	for _, cb := range lm.callbacks {
		cb(msg, level)
	}
}

// Close 关闭日志文件
func (lm *LogManager) Close() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.logFile != nil {
		_ = lm.logFile.Close()
		lm.logFile = nil
	}
}

// ── 上下文键 ────────────────────────────────────────────────────────

type logCtxKey struct{}

// WithLogAttrs 将属性附加到上下文，供日志使用
func WithLogAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	return context.WithValue(ctx, logCtxKey{}, attrs)
}
