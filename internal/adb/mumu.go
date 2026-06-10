package adb

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
)

// MuMuHelper MuMu 模拟器管理
type MuMuHelper struct {
	mu     sync.Mutex
	adbPath string
	exePath string
}

// NewMuMuHelper 创建 MuMu 模拟器管理器
func NewMuMuHelper(adbPath, mumuExePath string) *MuMuHelper {
	return &MuMuHelper{
		adbPath: adbPath,
		exePath: mumuExePath,
	}
}

// DefaultMuMu12AdbPaths MuMu 12 默认 adb 路径候选项
var DefaultMuMu12AdbPaths = []string{
	`D:\MuMuPlayer\nx_device\12.0\shell\adb.exe`,
	`C:\MuMuPlayer\nx_device\12.0\shell\adb.exe`,
	`C:\Program Files\MuMuPlayer\nx_device\12.0\shell\adb.exe`,
}

// DefaultMuMu12ExePaths MuMu 12 默认安装路径候选项
var DefaultMuMu12ExePaths = []string{
	`D:\MuMuPlayer\nx_device\12.0\`,
	`C:\MuMuPlayer\nx_device\12.0\`,
	`C:\Program Files\MuMuPlayer\nx_device\12.0\`,
}

// FindAdb 自动查找 adb 路径
func (m *MuMuHelper) FindAdb() string {
	if m.adbPath != "" {
		return m.adbPath
	}
	for _, p := range DefaultMuMu12AdbPaths {
		if fileExists(p) {
			m.adbPath = p
			return p
		}
	}
	return "adb"
}

// FindMuMuManager 查找 MuMuManager.exe 路径
func (m *MuMuHelper) FindMuMuManager() string {
	basePaths := []string{}
	if m.exePath != "" {
		basePaths = append(basePaths, m.exePath)
	}
	basePaths = append(basePaths, DefaultMuMu12ExePaths...)

	for _, base := range basePaths {
		candidate := filepath.Join(base, "nx_main", "MuMuManager.exe")
		if fileExists(candidate) {
			return candidate
		}
	}
	return ""
}

// SetGPS 通过 MuMuManager 设置 GPS 定位
// 返回 (成功, 消息)
func (m *MuMuHelper) SetGPS(latitude, longitude float64) (bool, string) {
	if latitude == 0 && longitude == 0 {
		return true, "GPS 未配置，跳过"
	}

	manager := m.FindMuMuManager()
	if manager == "" {
		return false, "未找到 MuMuManager.exe"
	}

	cmd := exec.Command(manager, "control", "-v", "0", "tool", "location",
		"-lat", fmt.Sprintf("%f", latitude),
		"-lon", fmt.Sprintf("%f", longitude),
	)

	hideWindow(cmd)

	output, err := cmd.CombinedOutput()
	outputStr := string(output)
	if err != nil {
		slog.Warn("GPS 设置失败", "error", err, "output", outputStr)
		return false, fmt.Sprintf("GPS 设置失败: %v", err)
	}

	slog.Info("GPS 已设置", "latitude", latitude, "longitude", longitude)
	return true, ""
}

// StartMuMu 启动 MuMu 模拟器（通过 MuMuManager）
func (m *MuMuHelper) StartMuMu() (bool, string) {
	manager := m.FindMuMuManager()
	if manager == "" {
		return false, "未找到 MuMuManager.exe"
	}

	cmd := exec.Command(manager, "launch", "0") // 启动第 0 个模拟器
	hideWindow(cmd)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("启动 MuMu 失败: %v", err)
	}
	slog.Info("MuMu 模拟器已启动")
	return true, string(output)
}

// StopMuMu 停止 MuMu 模拟器
func (m *MuMuHelper) StopMuMu() (bool, string) {
	manager := m.FindMuMuManager()
	if manager == "" {
		return false, "未找到 MuMuManager.exe"
	}

	cmd := exec.Command(manager, "control", "-v", "0", "stop")
	hideWindow(cmd)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("停止 MuMu 失败: %v", err)
	}
	slog.Info("MuMu 模拟器已停止")
	return true, string(output)
}

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
