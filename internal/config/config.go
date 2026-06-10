// Package config 提供配置管理（内嵌默认 + default.json 合并 + user_config.json 覆盖）
package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// ── 子配置结构 ──────────────────────────────────────────────────────

type MuMuConfig struct {
	Host         string  `json:"host"`
	Port         int     `json:"port"`
	AdbPath      string  `json:"adb_path"`
	MuMuExePath  string  `json:"mumu_exe_path"`
	GpsLatitude  float64 `json:"gps_latitude"`
	GpsLongitude float64 `json:"gps_longitude"`
}

type AppConfig struct {
	PackageName string `json:"package_name"`
	Activity    string `json:"activity"`
}

type HolidayConfig struct {
	SkipWeekend   bool     `json:"skip_weekend"`
	SkipHoliday   bool     `json:"skip_holiday"`
	ExtraWorkdays []string `json:"extra_workdays"`
	ExtraHolidays []string `json:"extra_holidays"`
}

type NotificationConfig struct {
	Enabled   bool   `json:"enabled"`
	Webhook   string `json:"webhook"`
	VerifyTLS bool   `json:"verify_tls"`
}

type UpdateConfig struct {
	ManifestURL        string `json:"manifest_url"`
	AutoCheckOnStartup bool   `json:"auto_check_on_startup"`
}

type RandomDelayConfig struct {
	MinSeconds int `json:"min_seconds"`
	MaxSeconds int `json:"max_seconds"`
}

type AppStateConfig struct {
	AutoConnect                      bool `json:"auto_connect"`
	AutoStart                        bool `json:"auto_start"`
	KeepAliveEnabled                 bool `json:"keep_alive_enabled"`
	RecoveryBaseBackoffSeconds       int  `json:"recovery_base_backoff_seconds"`
	RecoveryMaxBackoffSeconds        int  `json:"recovery_max_backoff_seconds"`
	RecoveryMaxFailures              int  `json:"recovery_max_failures"`
	RecoveryPauseMinutesAfterMax     int  `json:"recovery_pause_minutes_after_max_failures"`
	RecoveryQuietHoursEnabled        bool `json:"recovery_quiet_hours_enabled"`
	RecoveryQuietStartHour           int  `json:"recovery_quiet_start_hour"`
	RecoveryQuietEndHour             int  `json:"recovery_quiet_end_hour"`
}

type MakeupWindowConfig struct {
	MorningSignin    []int `json:"morning_signin"`
	MorningSignout   []int `json:"morning_signout"`
	AfternoonSignin  []int `json:"afternoon_signin"`
	AfternoonSignout []int `json:"afternoon_signout"`
}

type NetworkProbeConfig struct {
	Targets        [][]interface{} `json:"targets"`
	TimeoutSeconds int             `json:"timeout_seconds"`
}

type AdvancedConfig struct {
	SessionTTLSeconds   int                `json:"session_ttl_seconds"`
	MisfireGraceSeconds int                `json:"misfire_grace_seconds"`
	NetworkProbe        NetworkProbeConfig `json:"network_probe"`
}

// Config 顶层配置结构
type Config struct {
	MuMu         MuMuConfig         `json:"mumu"`
	Checkin      map[string]CheckinEntry `json:"checkin"`
	App          AppConfig          `json:"app"`
	Holiday      HolidayConfig      `json:"holiday"`
	Notification NotificationConfig `json:"notification"`
	Update       UpdateConfig       `json:"update"`
	RandomDelay  RandomDelayConfig  `json:"random_delay"`
	AppState     AppStateConfig     `json:"app_state"`
	MakeupWindow MakeupWindowConfig `json:"makeup_window"`
	Advanced     AdvancedConfig     `json:"advanced"`
}

type CheckinEntry struct {
	Enabled   bool     `json:"enabled"`
	TimeRange []string `json:"time_range"`
	Label     string   `json:"label"`
}

// ── 默认值 ──────────────────────────────────────────────────────────

var defaultPublicManifestURL = "https://raw.githubusercontent.com/juice4927/tongkatong-update/main/version.json"

func defaultConfig() *Config {
	return &Config{
		MuMu: MuMuConfig{
			Host: "127.0.0.1", Port: 5555,
		},
		Checkin: map[string]CheckinEntry{
			"morning_signin":    {Enabled: true, TimeRange: []string{"07:20", "07:55"}, Label: "上午签到"},
			"morning_signout":   {Enabled: true, TimeRange: []string{"11:35", "12:00"}, Label: "上午签退"},
			"afternoon_signin":  {Enabled: true, TimeRange: []string{"13:10", "13:25"}, Label: "下午签到"},
			"afternoon_signout": {Enabled: true, TimeRange: []string{"17:10", "17:30"}, Label: "下午签退"},
		},
		App: AppConfig{PackageName: "com.tencent.weworklocal"},
		Holiday: HolidayConfig{
			SkipWeekend: true, SkipHoliday: true,
		},
		Notification: NotificationConfig{Enabled: false, VerifyTLS: true},
		Update: UpdateConfig{
			ManifestURL:        defaultPublicManifestURL,
			AutoCheckOnStartup: true,
		},
		RandomDelay: RandomDelayConfig{MinSeconds: 1, MaxSeconds: 5},
		AppState: AppStateConfig{
			KeepAliveEnabled:             true,
			RecoveryBaseBackoffSeconds:   5,
			RecoveryMaxBackoffSeconds:    300,
			RecoveryMaxFailures:          20,
			RecoveryPauseMinutesAfterMax: 30,
		},
		MakeupWindow: MakeupWindowConfig{
			MorningSignin:    []int{4, 0, 8, 0},
			MorningSignout:   []int{11, 30, 13, 30},
			AfternoonSignin:  []int{11, 30, 13, 30},
			AfternoonSignout: []int{17, 0, 28, 0},
		},
		Advanced: AdvancedConfig{
			SessionTTLSeconds:   300,
			MisfireGraceSeconds: 300,
			NetworkProbe: NetworkProbeConfig{
				Targets: [][]interface{}{
					{"223.5.5.5", 53},
					{"114.114.114.114", 53},
					{"1.1.1.1", 53},
					{"8.8.8.8", 53},
				},
				TimeoutSeconds: 5,
			},
		},
	}
}

// ── ConfigManager ───────────────────────────────────────────────────

type ConfigManager struct {
	mu         sync.RWMutex
	configDir  string
	configFile string
	cfg        *Config
}

// NewConfigManager 创建配置管理器
func NewConfigManager(configDir string) *ConfigManager {
	cm := &ConfigManager{
		configDir:  configDir,
		configFile: filepath.Join(configDir, "user_config.json"),
	}
	cm.cfg = cm.loadMerged()
	return cm
}

// Config 返回当前配置（线程安全）
func (cm *ConfigManager) Config() *Config {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return cm.cfg
}

// Reload 重新加载配置
func (cm *ConfigManager) Reload() {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.cfg = cm.loadMerged()
}

// Save 保存当前配置到 user_config.json
func (cm *ConfigManager) Save() error {
	cm.mu.RLock()
	cfg := cm.cfg
	cm.mu.RUnlock()
	return cm.writeConfig(cfg)
}

// SaveConfig 保存指定配置
func (cm *ConfigManager) SaveConfig(cfg *Config) error {
	cm.mu.Lock()
	cm.cfg = cfg
	cm.mu.Unlock()
	return cm.writeConfig(cfg)
}

// ConfigDir 返回配置目录
func (cm *ConfigManager) ConfigDir() string {
	return cm.configDir
}

func (cm *ConfigManager) writeConfig(cfg *Config) error {
	_ = os.MkdirAll(cm.configDir, 0755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	// 原子写入
	tmpPath := cm.configFile + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("写入临时配置失败: %w", err)
	}
	if err := os.Rename(tmpPath, cm.configFile); err != nil {
		return fmt.Errorf("替换配置文件失败: %w", err)
	}
	return nil
}

// ── JSON 层级合并 ───────────────────────────────────────────────────

func (cm *ConfigManager) loadMerged() *Config {
	cfg := defaultConfig()

	mergedMap := make(map[string]json.RawMessage)
	builtinData, _ := json.Marshal(cfg)
	json.Unmarshal(builtinData, &mergedMap)

	if data, err := os.ReadFile(filepath.Join(cm.configDir, "default.json")); err == nil {
		var overrideMap map[string]json.RawMessage
		if err := json.Unmarshal(data, &overrideMap); err == nil {
			deepMergeMap(mergedMap, overrideMap)
		}
	}

	userExists := true
	if data, err := os.ReadFile(cm.configFile); err == nil {
		var overrideMap map[string]json.RawMessage
		if err := json.Unmarshal(data, &overrideMap); err == nil {
			deepMergeMap(mergedMap, overrideMap)
		}
	} else if os.IsNotExist(err) {
		userExists = false
	}

	mergedData, _ := json.Marshal(mergedMap)
	var result Config
	if err := json.Unmarshal(mergedData, &result); err != nil {
		slog.Error("配置合并解析失败，使用内置默认", "error", err)
		return cfg
	}

	if !userExists {
		cm.cfg = &result
		_ = cm.writeConfig(&result)
	}

	return &result
}

// deepMergeMap 深度合并两个 JSON map（override 覆盖 base 的对应 key）
func deepMergeMap(base, override map[string]json.RawMessage) {
	for k, v := range override {
		var baseVal, overrideVal map[string]json.RawMessage
		if baseRaw, ok := base[k]; ok {
			if err := json.Unmarshal(baseRaw, &baseVal); err == nil {
				if err := json.Unmarshal(v, &overrideVal); err == nil {
					deepMergeMap(baseVal, overrideVal)
					merged, _ := json.Marshal(baseVal)
					base[k] = merged
					continue
				}
			}
		}
		base[k] = v
	}
}
