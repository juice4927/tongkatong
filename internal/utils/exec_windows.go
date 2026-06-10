//go:build windows

package utils

import (
	"os/exec"
	"syscall"
)

// HideWindow 设置 Windows 隐藏窗口属性
func HideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
