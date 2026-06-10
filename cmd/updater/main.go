// tongkatong-updater - 通卡通自动更新启动器
//
//	由主程序启动，等待主程序退出后替换文件，然后启动新版本
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	source := flag.String("source", "", "更新包路径")
	target := flag.String("target", "", "目标 exe 路径")
	stateDir := flag.String("state-dir", "", "状态文件目录")
	version := flag.String("version", "", "目标版本")
	prevVersion := flag.String("prev-version", "", "当前版本")
	// passthroughArgs 收集传递给新版本的原始 CLI 参数
	var passthroughArgs []string
	flag.Func("passthrough-args", "透传原始 CLI 参数", func(s string) error {
		passthroughArgs = append(passthroughArgs, s)
		return nil
	})
	flag.Parse()

	if *source == "" || *target == "" || *stateDir == "" {
		fmt.Fprintln(os.Stderr, "Usage: updater --source <file> --target <exe> --state-dir <dir> --version <ver> --prev-version <ver>")
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	logger.Info("更新启动器开始工作",
		"source", *source,
		"target", *target,
		"version", *version,
	)

	writeState := func(status, detail string) {
		state := map[string]string{
			"status":           status,
			"target_version":   *version,
			"previous_version": *prevVersion,
			"detail":           detail,
			"updated_at":       time.Now().Format("2006-01-02 15:04:05"),
		}
		data, _ := json.MarshalIndent(state, "", "  ")
		_ = os.WriteFile(filepath.Join(*stateDir, "update_state.json"), data, 0644)
	}

	writeState("waiting_exit", "等待旧进程退出...")

	// 等待旧进程退出（最多 20 秒）
	// 通过检查当前进程的父进程是否退出
	for i := 0; i < 40; i++ {
		// 检查目标文件是否可写（进程退出后文件不再被锁定）
		f, err := os.OpenFile(*target, os.O_WRONLY|os.O_APPEND, 0)
		if err == nil {
			f.Close()
			break
		}
		time.Sleep(500 * time.Millisecond)
		logger.Info("等待旧进程退出...", "attempt", i+1)
	}

	// 备份旧文件
	backupPath := *target + ".old"
	_ = os.Remove(backupPath)
	if err := copyFile(*target, backupPath); err != nil {
		logger.Error("备份失败", "error", err)
		writeState("failed", "备份失败: "+err.Error())
		os.Exit(1)
	}

	// 替换文件
	if err := copyFile(*source, *target); err != nil {
		logger.Error("替换文件失败", "error", err)
		// 回滚
		_ = copyFile(backupPath, *target)
		writeState("rolled_back", "替换失败，已回滚: "+err.Error())
		os.Exit(1)
	}

	logger.Info("文件替换成功，启动新版本...")
	writeState("success", "更新成功")

	// 启动新版本（透传原始 CLI 参数）
	cmd := exec.Command(*target, passthroughArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		logger.Error("启动新版本失败", "error", err)
		// 回滚
		_ = copyFile(backupPath, *target)
		writeState("rolled_back", "启动新版本失败，已回滚: "+err.Error())
		os.Exit(1)
	}

	logger.Info("新版本已启动")

	// 清理备份
	_ = os.Remove(backupPath)
	_ = os.Remove(*source)

	// 自删除：启动完成后退出
	os.Exit(0)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	// 重试写入（最多 5 次，间隔 500ms），应对 Windows 文件锁定
	var lastErr error
	for i := 0; i < 5; i++ {
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		if err := os.WriteFile(dst, data, 0644); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("文件写入失败（已重试5次）: %w", lastErr)
}
