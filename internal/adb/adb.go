// Package adb 提供 ADB 设备管理功能
package adb

import (
	"bytes"
	"context"
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

// runCommand 执行 adb 命令，返回 (success, stdout_string)
func (a *ADBHelper) runCommand(args []string, timeout time.Duration) (bool, string) {
	ok, stdout, stderr := a.runCommandRaw(args, timeout)
	if !ok && len(stderr) > 0 {
		slog.Debug("ADB 命令失败", "args", args, "stderr", string(stderr))
	}
	return ok, string(stdout)
}

// runCommandRaw 执行 adb 命令，返回 (success, stdout_bytes, stderr_bytes)
func (a *ADBHelper) runCommandRaw(args []string, timeout time.Duration) (bool, []byte, []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.adbPath, args...)
	hideWindow(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return false, nil, []byte(fmt.Sprintf("启动命令失败: %v", err))
	}

	err := cmd.Wait()
	if err != nil {
		if ctx.Err() != nil {
			return false, nil, []byte("命令超时")
		}
		return false, stdout.Bytes(), stderr.Bytes()
	}
	return true, stdout.Bytes(), stderr.Bytes()
}

// Version 获取 ADB 版本
func (a *ADBHelper) Version() string {
	ok, out := a.runCommand([]string{"version"}, 10*time.Second)
	if !ok {
		return ""
	}
	for _, line := range strings.Split(out, "\n") {
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
	for _, line := range strings.Split(out, "\n")[1:] {
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

// Shell 执行 shell 命令（返回 stdout 字符串）
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

// Screencap 截屏（返回原始 PNG 字节，不做字符串转换）
func (a *ADBHelper) Screencap(device string) ([]byte, error) {
	args := []string{}
	if device != "" {
		args = append(args, "-s", device)
	}
	args = append(args, "shell", "screencap -p")

	ok, stdout, stderr := a.runCommandRaw(args, 15*time.Second)
	if !ok {
		return nil, fmt.Errorf("截屏失败: %s", string(stderr))
	}
	// 清理可能附带的换行/空字节前缀
	data := bytes.TrimLeft(stdout, "\x00\r\n ")
	// 查找 PNG 文件头
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47}
	if idx := bytes.Index(data, pngHeader); idx > 0 {
		data = data[idx:]
	}
	if len(data) == 0 || !bytes.HasPrefix(data, pngHeader) {
		return nil, fmt.Errorf("截屏数据不是有效的 PNG（%d bytes）", len(data))
	}
	return data, nil
}

// DumpHierarchy 获取 UI hierarchy XML
func (a *ADBHelper) DumpHierarchy(device string) (string, error) {
	// 方式1：直接输出到 stdout（Android 7+ 支持 uiautomator dump /dev/tty）
	ok, stdout := a.Shell(device, "uiautomator dump /dev/tty", 15*time.Second)
	if ok && strings.Contains(stdout, "<") && strings.Contains(stdout, "node") {
		return stdout, nil
	}

	// 方式2：写文件再读取
	ok2, _ := a.Shell(device, "uiautomator dump /sdcard/ui.xml", 15*time.Second)
	if !ok2 {
		return "", fmt.Errorf("dump hierarchy 失败")
	}

	// 从设备读取文件
	readArgs := []string{}
	if device != "" {
		readArgs = append(readArgs, "-s", device)
	}
	readArgs = append(readArgs, "shell", "cat /sdcard/ui.xml")

	ok3, out3 := a.runCommand(readArgs, 10*time.Second)
	if ok3 {
		return out3, nil
	}
	return "", fmt.Errorf("读取 hierarchy XML 失败: %s", out3)
}
