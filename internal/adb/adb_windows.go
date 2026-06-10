//go:build windows

package adb

import (
	"os/exec"
	"syscall"
)

func init() {
	hideWindow = func(cmd *exec.Cmd) {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000, // CREATE_NO_WINDOW
		}
	}
}
