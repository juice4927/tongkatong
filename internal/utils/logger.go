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
	globalOnce    sync.Once
)

// GetLogManager 返回全局 LogManager 实例（线程安全单例）
func GetLogManager() *LogManager {
	globalOnce.Do(func() {
		globalManager = &LogManager{}
	})
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
		// 关闭旧日志文件（防止多次调用 SetupLogging 泄漏句柄）
		if lm.logFile != nil {
			_ = lm.logFile.Close()
		}
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

	handler := &callbackHandler{
		handler: slog.NewTextHandler(writer, &slog.HandlerOptions{
			Level: logLevel,
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == slog.TimeKey {
					return slog.String("time", a.Value.Time().Format("2006-01-02 15:04:05"))
				}
				return a
			},
		}),
		manager: lm,
	}

	lm.logger = slog.New(handler)
	slog.SetDefault(lm.logger)
	lm.mu.Unlock()

	slog.Info("日志系统初始化完成", "logDir", logDir, "level", level)
	return nil
}

// callbackHandler 包装 slog.Handler，在每次日志写入时同时通知 GUI 回调
type callbackHandler struct {
	handler slog.Handler
	manager *LogManager
}

func (h *callbackHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func (h *callbackHandler) Handle(ctx context.Context, r slog.Record) error {
	// 通知所有 GUI 回调
	h.manager.mu.RLock()
	msg := r.Message
	levelStr := r.Level.String()
	for _, cb := range h.manager.callbacks {
		cb(msg, levelStr)
	}
	h.manager.mu.RUnlock()

	return h.handler.Handle(ctx, r)
}

func (h *callbackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &callbackHandler{
		handler: h.handler.WithAttrs(attrs),
		manager: h.manager,
	}
}

func (h *callbackHandler) WithGroup(name string) slog.Handler {
	return &callbackHandler{
		handler: h.handler.WithGroup(name),
		manager: h.manager,
	}
}

// AddCallback 注册日志回调
func (lm *LogManager) AddCallback(cb LogCallback) {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	lm.callbacks = append(lm.callbacks, cb)
}

// Close 关闭日志文件并清理资源
func (lm *LogManager) Close() {
	lm.mu.Lock()
	defer lm.mu.Unlock()
	if lm.logFile != nil {
		_ = lm.logFile.Close()
		lm.logFile = nil
	}
	lm.callbacks = nil
}
