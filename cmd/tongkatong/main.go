// tongkatong - 通卡通 自动化打卡工具
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/automator"
	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/holiday"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/utils"
)

func getDefaultConfigDir() string {
	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, "config")
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
	}
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "config")
	}
	return "config"
}

func main() {
	headless := flag.Bool("headless", false, "以无界面模式运行")
	configDir := flag.String("config", "", "配置文件目录")
	flag.Parse()

	cfgDir := *configDir
	if cfgDir == "" {
		cfgDir = getDefaultConfigDir()
	}
	_ = os.MkdirAll(cfgDir, 0755)

	logDir := filepath.Join(cfgDir, "..", "logs")
	_ = utils.SetupLogging(logDir, "INFO", true)
	slog.Info("启动", "app", models.AppName, "version", models.Version, "build", models.BuildDate)

	cm := config.NewConfigManager(cfgDir)
	cfg := cm.Config()
	slog.Info("配置加载完成", "adb_host", cfg.MuMu.Host, "adb_port", cfg.MuMu.Port)

	// 初始化 ADB
	adbPath := cfg.MuMu.AdbPath
	adbHelper := adb.NewADBHelper(adbPath)
	devicePool := adb.NewDevicePool(adbHelper, time.Duration(cfg.Advanced.SessionTTLSeconds)*time.Second)
	mumu := adb.NewMuMuHelper(cfg.MuMu.AdbPath, cfg.MuMu.MuMuExePath)

	foundAdb := mumu.FindAdb()
	if foundAdb != "" && foundAdb != "adb" && foundAdb != cfg.MuMu.AdbPath {
		slog.Info("自动发现 MuMu ADB", "path", foundAdb)
		adbHelper.SetADBPath(foundAdb)
		mumu = adb.NewMuMuHelper(foundAdb, cfg.MuMu.MuMuExePath)
	}

	version := adbHelper.Version()
	if version == "" {
		slog.Warn("ADB 不可用，请检查 adb 路径")
	} else {
		slog.Info("ADB 版本", "version", version)
	}

	devices := adbHelper.Devices()
	slog.Info("已连接设备", "count", len(devices))
	for _, d := range devices {
		slog.Info("  设备", "serial", d.Serial, "status", d.Status)
	}

	fmt.Println("\n通卡通 v" + models.Version + " — 启动完成")
	fmt.Println("配置文件目录:", cfgDir)
	fmt.Printf("设备数量: %d\n", len(devices))

	if *headless {
		slog.Info("Headless 模式启动，初始化自动化引擎...")

		// 连接设备（多端口尝试）
		connected := false
		for _, port := range []int{cfg.MuMu.Port, 7555, 5555} {
			if port <= 0 {
				continue
			}
			ok, msg := adbHelper.Connect(cfg.MuMu.Host, port)
			if ok {
				slog.Info("设备连接成功", "port", port, "msg", msg)
				connected = true
				break
			}
			slog.Warn("端口连接失败", "port", port, "msg", msg)
		}

		// 所有端口失败 → 自动启动 MuMu
		if !connected {
			slog.Info("所有端口连接失败，尝试启动 MuMu 模拟器...")
			launched, launchMsg := mumu.LaunchMuMu(adbHelper, cfg.MuMu.Host, cfg.MuMu.Port, 60)
			if !launched {
				slog.Error("MuMu 启动失败", "msg", launchMsg)
				fmt.Println("设备连接失败:", launchMsg)
				os.Exit(1)
			}
			// 重试连接
			for _, port := range []int{cfg.MuMu.Port, 7555, 5555} {
				if port <= 0 {
					continue
				}
				if ok, _ := adbHelper.Connect(cfg.MuMu.Host, port); ok {
					slog.Info("MuMu 启动后连接成功", "port", port)
					connected = true
					break
				}
			}
		}

		if !connected {
			slog.Error("设备连接失败：所有端口均不可达")
			fmt.Println("设备连接失败：请确保 MuMu 模拟器已启动且已开启 ADB 调试")
			os.Exit(1)
		}
		fmt.Println("设备已连接:", cfg.MuMu.Host, cfg.MuMu.Port)

		// 初始化自动化引擎
		uia := automator.NewUIAutomator2Impl(
			cfg.MuMu.Host, cfg.MuMu.Port,
			adbHelper.GetADBPath(),
			cfg.App.PackageName,
			devicePool,
		)
		uia.SetMuMuHelper(mumu)
		hc := holiday.NewHolidayChecker(
			cfg.Holiday.SkipWeekend,
			cfg.Holiday.SkipHoliday,
			cfg.Holiday.ExtraWorkdays,
			cfg.Holiday.ExtraHolidays,
		)

		// 初始化调度器
		orc := automator.NewCheckinOrchestrator(uia, hc, cm, filepath.Dir(cfgDir))
		if !orc.Initialize() {
			slog.Error("调度器初始化失败")
			os.Exit(1)
		}

		orc.Start()
		defer orc.Stop()
		slog.Info("Headless 模式已启动，等待定时任务...")
		fmt.Println("调度器已启动，等待定时打卡...")
	}

	// 等待退出信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	slog.Info("正在退出...")
	fmt.Println("\n正在退出...")
}
