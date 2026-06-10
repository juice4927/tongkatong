package config_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/juice4927/tongkatong/internal/config"
)

func TestConfigDefaults(t *testing.T) {
	cm := config.NewConfigManager(".")
	cfg := cm.Config()

	if cfg.MuMu.Host != "127.0.0.1" {
		t.Errorf("default host should be 127.0.0.1, got %s", cfg.MuMu.Host)
	}
	if cfg.MuMu.Port != 5555 {
		t.Errorf("default port should be 5555, got %d", cfg.MuMu.Port)
	}
	if cfg.App.PackageName != "com.tencent.weworklocal" {
		t.Errorf("default package should be com.tencent.weworklocal, got %s", cfg.App.PackageName)
	}
	if !cfg.Holiday.SkipWeekend {
		t.Error("default should skip weekend")
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	cm := config.NewConfigManager(dir)

	cfg := cm.Config()
	cfg.MuMu.Host = "192.168.1.100"
	cfg.MuMu.Port = 1234

	if err := cm.SaveConfig(cfg); err != nil {
		t.Fatalf("save config: %v", err)
	}

	// 重新加载
	cm2 := config.NewConfigManager(dir)
	cfg2 := cm2.Config()

	if cfg2.MuMu.Host != "192.168.1.100" {
		t.Errorf("loaded host should be 192.168.1.100, got %s", cfg2.MuMu.Host)
	}
	if cfg2.MuMu.Port != 1234 {
		t.Errorf("loaded port should be 1234, got %d", cfg2.MuMu.Port)
	}
}

func TestConfigWithDefaultJSON(t *testing.T) {
	dir := t.TempDir()

	// 创建 default.json
	defaultJSON := `{
		"mumu": {
			"host": "10.0.0.1",
			"adb_path": "D:\\adb.exe"
		}
	}`
	_ = os.WriteFile(filepath.Join(dir, "default.json"), []byte(defaultJSON), 0644)

	cm := config.NewConfigManager(dir)
	cfg := cm.Config()

	if cfg.MuMu.Host != "10.0.0.1" {
		t.Errorf("merged host should be 10.0.0.1, got %s", cfg.MuMu.Host)
	}
	if cfg.MuMu.AdbPath != "D:\\adb.exe" {
		t.Errorf("merged adb_path should be D:\\adb.exe, got %s", cfg.MuMu.AdbPath)
	}
	// 内置默认值应该保留
	if cfg.App.PackageName != "com.tencent.weworklocal" {
		t.Errorf("default package should be carried over, got %s", cfg.App.PackageName)
	}
}

func TestConfigJSONRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cm := config.NewConfigManager(dir)

	cfg := cm.Config()
	data, _ := json.MarshalIndent(cfg, "", "  ")

	var decoded config.Config
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json roundtrip: %v", err)
	}

	if decoded.MuMu.Host != cfg.MuMu.Host {
		t.Errorf("json roundtrip host mismatch")
	}
}
