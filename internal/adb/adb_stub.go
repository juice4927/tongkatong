//go:build !windows

package adb

import (
	"os/exec"
)

func hideWindow(cmd *exec.Cmd) {
	// 非 Windows 平台无需隐藏窗口
}
