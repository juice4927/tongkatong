// Package adb 提供 ADB 设备管理功能
package adb

import (
	"bufio"
	"bytes"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DeviceInfo 设备信息
type DeviceInfo struct {
	Serial string
	Status string
}

// DeviceSession ADB 设备会话
type DeviceSession struct {
	Address   string
	CreatedAt time.Time
	TTL       time.Duration
}

func (s *DeviceSession) IsExpired() bool {
	return time.Since(s.CreatedAt) > s.TTL
}

// ADBHelper ADB 辅助类
type ADBHelper struct {
	adbPath string
	mu      sync.Mutex
}

// NewADBHelper 创建 ADBHelper，adbPath 为空时使用系统 PATH 中的 adb
func NewADBHelper(adbPath string) *ADBHelper {
	if adbPath == "" {
		adbPath = "adb"
	}
	return &ADBHelper{adbPath: adbPath}
}

// SetADBPath 设置 ADB 路径
func (a *ADBHelper) SetADBPath(path string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.adbPath = path
}

// GetADBPath 获取当前 ADB 路径
func (a *ADBHelper) GetADBPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.adbPath
}

// runCommand 执行 adb 命令，返回 (success, stdout)
func (a *ADBHelper) runCommand(args []string, timeout time.Duration) (bool, string) {
	cmd := exec.Command(a.adbPath, args...)

	// Windows: 隐藏窗口
	hideWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return false, fmt.Sprintf("启动命令失败: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return false, strings.TrimSpace(stderr.String())
		}
		return true, strings.TrimSpace(stdout.String())
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return false, "命令超时"
	}
}

// Version 获取 ADB 版本
func (a *ADBHelper) Version() string {
	ok, out := a.runCommand([]string{"version"}, 10*time.Second)
	if !ok {
		return ""
	}
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "Android Debug Bridge") {
			return line
		}
	}
	return ""
}

// Devices 获取已连接设备列表
func (a *ADBHelper) Devices() []DeviceInfo {
	ok, out := a.runCommand([]string{"devices", "-l"}, 10*time.Second)
	if !ok {
		return nil
	}

	var devices []DeviceInfo
	lines := strings.Split(out, "\n")
	for _, line := range lines[1:] { // 跳过标题行
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			devices = append(devices, DeviceInfo{
				Serial: parts[0],
				Status: parts[1],
			})
		}
	}
	return devices
}

// Connect 连接到远程设备
func (a *ADBHelper) Connect(host string, port int) (bool, string) {
	address := fmt.Sprintf("%s:%d", host, port)
	ok, out := a.runCommand([]string{"connect", address}, 15*time.Second)
	if ok {
		lower := strings.ToLower(out)
		if strings.Contains(lower, "connected") || strings.Contains(lower, "already connected") {
			slog.Info("已连接到设备", "address", address)
			return true, out
		}
	}
	return false, out
}

// Disconnect 断开设备连接
func (a *ADBHelper) Disconnect(host string, port int) {
	address := fmt.Sprintf("%s:%d", host, port)
	a.runCommand([]string{"disconnect", address}, 5*time.Second)
	slog.Info("已断开设备", "address", address)
}

// Shell 执行 shell 命令
func (a *ADBHelper) Shell(device string, command string, timeout time.Duration) (bool, string) {
	args := []string{}
	if device != "" {
		args = append(args, "-s", device)
	}
	args = append(args, "shell", command)

	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return a.runCommand(args, timeout)
}

// Screencap 截屏（返回 PNG 字节）
func (a *ADBHelper) Screencap(device string) ([]byte, error) {
	ok, out := a.Shell(device, "screencap -p 2>/dev/null", 15*time.Second)
	if !ok {
		return nil, fmt.Errorf("截屏失败: %s", out)
	}
	// adb shell screencap 输出可能包含终端控制字符，需要清理
	clean := cleanScreencapOutput([]byte(out))
	return clean, nil
}

// DumpHierarchy 获取 UI hierarchy XML
func (a *ADBHelper) DumpHierarchy(device string) (string, error) {
	// 先执行 dump
	ok, err := a.Shell(device, "uiautomator dump /dev/tty 2>/dev/null || uiautomator dump /sdcard/ui.xml 2>/dev/null", 15*time.Second)
	if !ok || strings.Contains(err, "error") {
		// 尝试备用方式
		ok2, err2 := a.Shell(device, "uiautomator dump /sdcard/ui.xml", 15*time.Second)
		if !ok2 {
			return "", fmt.Errorf("dump hierarchy 失败: %s", err2)
		}
		_ = err // 忽略
		_ = err2

		// 从设备拉取文件
		ok3, out3 := a.runCommand([]string{"-s", device, "shell", "cat", "/sdcard/ui.xml"}, 10*time.Second)
		if ok3 {
			return out3, nil
		}
		return "", fmt.Errorf("读取 hierarchy XML 失败: %s", out3)
	}
	return err, nil
}

// 清理 screencap 输出中的终端控制字符
func cleanScreencapOutput(data []byte) []byte {
	// 查找 PNG 文件头
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47}
	idx := bytes.Index(data, pngHeader)
	if idx > 0 {
		return data[idx:]
	}
	return data
}

// ── Windows 隐藏窗口 ────────────────────────────────────────────────

var hideWindow = func(cmd *exec.Cmd) {
	// 在 Windows 上设置 CREATE_NO_WINDOW
	// 通过 build tags 实现，详见 adb_windows.go
}
