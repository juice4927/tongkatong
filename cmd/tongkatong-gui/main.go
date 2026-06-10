// Package main Wails GUI 入口
package main

import (
	"embed"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/utils"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

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
	// 配置目录
	cfgDir := getDefaultConfigDir()
	_ = os.MkdirAll(cfgDir, 0755)

	// 日志
	logDir := filepath.Join(cfgDir, "..", "logs")
	_ = utils.SetupLogging(logDir, "INFO", true)
	slog.Info(models.AppName+" v"+models.Version, "config", cfgDir)

	// 加载配置
	cm := config.NewConfigManager(cfgDir)

	// 创建 App
	app := NewApp(cm, cfgDir)

	err := wails.Run(&options.App{
		Title:     models.AppName + " v" + models.Version,
		Width:     1024,
		Height:    768,
		MinWidth:  860,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		OnDomReady: app.domReady,
		// Window: 关闭时最小化到托盘而非退出
		OnBeforeClose: app.onBeforeClose,
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		slog.Error("GUI 启动失败", "error", err)
		os.Exit(1)
	}
}
