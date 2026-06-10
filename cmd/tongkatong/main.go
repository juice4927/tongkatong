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

		// 连接设备
		ok, msg := adbHelper.Connect(cfg.MuMu.Host, cfg.MuMu.Port)
		if !ok {
			slog.Error("设备连接失败", "msg", msg)
			fmt.Println("设备连接失败:", msg)
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
