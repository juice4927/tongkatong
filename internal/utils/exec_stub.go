//go:build !windows

package utils

import "os/exec"

// HideWindow 非 Windows 平台空实现
func HideWindow(cmd *exec.Cmd) {
	// no-op on non-Windows
}
