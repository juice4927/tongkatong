package adb

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
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
	`D:\MuMuPlayer`,
	`C:\MuMuPlayer`,
	`E:\MuMuPlayer`,
	`F:\MuMuPlayer`,
	`D:\Program Files\Netease\MuMuPlayer-12.0`,
	`C:\Program Files\Netease\MuMuPlayer-12.0`,
	`C:\Program Files (x86)\Netease\MuMuPlayer-12.0`,
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
	// 更广泛的搜索：检查 basePath 下的常见 adb 位置
	for _, base := range DefaultMuMu12ExePaths {
		for _, sub := range []string{"shell", "nx_main", "nx_device/12.0/shell"} {
			candidate := filepath.Join(base, sub, "adb.exe")
			if fileExists(candidate) {
				m.adbPath = candidate
				return candidate
			}
		}
	}
	return "adb"
}

// FindMuMuManager 查找 MuMuManager.exe 路径
func (m *MuMuHelper) FindMuMuManager() string {
	// 如果用户配置了路径，优先使用
	if m.exePath != "" {
		candidate := filepath.Join(m.exePath, "nx_main", "MuMuManager.exe")
		if fileExists(candidate) {
			return candidate
		}
	}

	// 更广泛的搜索
	for _, base := range DefaultMuMu12ExePaths {
		for _, sub := range []string{"nx_main", "nx_device/12.0/nx_main"} {
			candidate := filepath.Join(base, sub, "MuMuManager.exe")
			if fileExists(candidate) {
				return candidate
			}
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

func fileExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// LaunchMuMu 启动 MuMu 模拟器并等待设备就绪
// 返回 (启动成功, 消息)
func (m *MuMuHelper) LaunchMuMu(adbHelper *ADBHelper, host string, port int, waitSeconds int) (bool, string) {
	manager := m.FindMuMuManager()
	if manager == "" {
		return false, "未找到 MuMuManager.exe，请在设置中配置 MuMu 安装路径"
	}

	slog.Info("使用 MuMuManager 启动模拟器实例", "manager", manager)
	// 优先用 MuMuManager control launch
	cmd := exec.Command(manager, "control", "-v", "0", "launch")
	hideWindow(cmd)
	_ = cmd.Run()

	// 等待设备出现，每 5 秒重试一次 adb connect
	slog.Info("等待 MuMu 启动就绪", "max_wait", waitSeconds)
	address := fmt.Sprintf("%s:%d", host, port)
	for i := 0; i < waitSeconds; i++ {
		time.Sleep(1 * time.Second)

		// 每 5 秒尝试主动连接
		if i%5 == 4 {
			adbHelper.Connect(host, port)
		}

		// 检查是否已有 device 状态的设备
		devices := adbHelper.Devices()
		for _, d := range devices {
			if d.Status == "device" {
				slog.Info("MuMu 设备已就绪", "elapsed_seconds", i+1)
				goto launched
			}
		}
	}
launched:

	// 最终手动 connect 一次
	ok, msg := adbHelper.Connect(host, port)
	if ok {
		// 关闭广告：返回桌面
		homeCmd := exec.Command(manager, "control", "-v", "0", "tool", "func", "-n", "go_home")
		hideWindow(homeCmd)
		_ = homeCmd.Run()
		slog.Info("已尝试关闭广告（返回桌面）")
		return true, "MuMu 已启动，连接成功: " + msg
	}
	return false, fmt.Sprintf("MuMu 启动超时（%d秒），请手动启动模拟器。已尝试连接 %s", waitSeconds, address)
}
