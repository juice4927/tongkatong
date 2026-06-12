//go:build windows

package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	regKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	regName    = "通卡通"
	taskName   = "TongKaTong-AutoStart"
)

func startupCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(`"%s" --start-hidden`, exe), nil
}

func IsEnabled() bool {
	if enabled, err := isTaskEnabled(); err == nil && enabled {
		return true
	}
	return isRegistryEnabled()
}

func SetEnabled(enable bool) error {
	command, err := startupCommand()
	if err != nil {
		return fmt.Errorf("获取启动命令失败: %w", err)
	}

	if enable {
		if err := createTask(command); err == nil {
			_ = setRegistryValue(command)
			return nil
		}
		if err := setRegistryValue(command); err != nil {
			return fmt.Errorf("设置开机自启失败: %w", err)
		}
		return nil
	}

	_ = deleteTask()
	_ = deleteRegistryValue()
	return nil
}

func isTaskEnabled() (bool, error) {
	output, err := exec.Command("schtasks", "/Query", "/TN", taskName, "/V", "/FO", "LIST").CombinedOutput()
	if err != nil {
		return false, err
	}
	text := string(output)
	return strings.Contains(text, "Ready") || strings.Contains(text, "准备就绪"), nil
}

func createTask(command string) error {
	cmd := exec.Command("schtasks", "/Create", "/TN", taskName, "/TR", command, "/SC", "ONLOGON", "/RL", "LIMITED", "/F")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func deleteTask() error {
	cmd := exec.Command("schtasks", "/Delete", "/TN", taskName, "/F")
	_ = cmd.Run()
	return nil
}

func isRegistryEnabled() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	_, _, err = key.GetStringValue(regName)
	return err == nil
}

func setRegistryValue(command string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, regKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(regName, command)
}

func deleteRegistryValue() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer key.Close()
	if err := key.DeleteValue(regName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
