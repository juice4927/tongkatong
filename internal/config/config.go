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

	// 对于每层文件，直接 JSON 解析出完整 Config，然后 field-by-field 合并
	applyFile := func(filePath string) {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return
		}
		// 转为完整 Config 再合并
		var fileCfg Config
		if err := json.Unmarshal(data, &fileCfg); err != nil {
			slog.Error("配置文件解析失败", "path", filePath, "error", err)
			return
		}
		// 只覆盖文件中显式指定的顶级字段
		var rawMap map[string]json.RawMessage
		if err := json.Unmarshal(data, &rawMap); err != nil {
			slog.Warn("配置文件 raw 解析失败，跳过合并", "path", filePath, "error", err)
			return
		}
		mergeInto(cfg, &fileCfg, rawMap)
	}

	applyFile(filepath.Join(cm.configDir, "default.json"))
	userExists := true
	if _, err := os.Stat(cm.configFile); err != nil {
		userExists = false
	} else {
		applyFile(cm.configFile)
	}

	if !userExists {
		_ = cm.writeConfig(cfg)
	}

	return cfg
}

// mergeInto 将 src 合并到 dst，只覆盖 rawMap 中显式指定的顶级字段
func mergeInto(dst, src *Config, rawMap map[string]json.RawMessage) {
	if _, ok := rawMap["mumu"]; ok { dst.MuMu = src.MuMu }
	if _, ok := rawMap["app"]; ok { dst.App = src.App }
	if _, ok := rawMap["holiday"]; ok { dst.Holiday = src.Holiday }
	if _, ok := rawMap["notification"]; ok { dst.Notification = src.Notification }
	if _, ok := rawMap["update"]; ok { dst.Update = src.Update }
	if _, ok := rawMap["random_delay"]; ok { dst.RandomDelay = src.RandomDelay }
	if _, ok := rawMap["app_state"]; ok { dst.AppState = src.AppState }
	if _, ok := rawMap["makeup_window"]; ok { dst.MakeupWindow = src.MakeupWindow }
	if _, ok := rawMap["advanced"]; ok { dst.Advanced = src.Advanced }
	// Checkin map：逐条合并（字段级别，防止只写 enabled 时丢失 time_range）
	if _, ok := rawMap["checkin"]; ok && src.Checkin != nil {
		if dst.Checkin == nil {
			dst.Checkin = make(map[string]CheckinEntry)
		}
		// 解析用户 JSON 中 checkin 各条目的原始内容，用于判断字段是否显式指定
		var rawCheckin map[string]json.RawMessage
		_ = json.Unmarshal(rawMap["checkin"], &rawCheckin)

		for k, v := range src.Checkin {
			if existing, exists := dst.Checkin[k]; exists {
				// 仅覆盖 src 中非零值的字段
				if v.Label != "" {
					existing.Label = v.Label
				}
				if len(v.TimeRange) > 0 {
					existing.TimeRange = v.TimeRange
				}
				// enabled 仅当用户 JSON 中显式写了该字段时才覆盖
				if rawEntry, hasRaw := rawCheckin[k]; hasRaw {
					var partial map[string]json.RawMessage
					if json.Unmarshal(rawEntry, &partial) == nil {
						if _, explicitEnabled := partial["enabled"]; explicitEnabled {
							existing.Enabled = v.Enabled
						}
					}
				}
				dst.Checkin[k] = existing
			} else {
				dst.Checkin[k] = v
			}
		}
	}
}
