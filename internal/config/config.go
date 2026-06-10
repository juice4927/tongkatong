// Package config 提供配置管理（内嵌默认 + default.json 合并 + user_config.json 覆盖）
package config

import (
	"encoding/json"
	"fmt"
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

func (cm *ConfigManager) loadMerged() *Config {
	// 1. 从内置默认开始
	cfg := defaultConfig()

	// 2. 尝试加载 default.json 覆盖
	if data, err := os.ReadFile(filepath.Join(cm.configDir, "default.json")); err == nil {
		var defCfg Config
		if err := json.Unmarshal(data, &defCfg); err == nil {
			cfg = merge(cfg, &defCfg)
		}
	}

	// 3. 用户配置覆盖
	userExists := true
	if data, err := os.ReadFile(cm.configFile); err == nil {
		var userCfg Config
		if err := json.Unmarshal(data, &userCfg); err == nil {
			cfg = merge(cfg, &userCfg)
		}
	} else if os.IsNotExist(err) {
		userExists = false
	}

	// 4. 首次运行：保存默认配置到 user_config.json
	if !userExists {
		_ = cm.writeConfig(cfg)
	}

	return cfg
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

// ── 深层合并 ────────────────────────────────────────────────────────

// merge 将 src 的非零值字段合并到 dst 中（dst 的零值字段被 src 覆盖）
func merge(dst, src *Config) *Config {
	// 逐字段手动合并，确保嵌套结构正确
	dst.MuMu = mergeMuMu(dst.MuMu, src.MuMu)
	dst.App = mergeApp(dst.App, src.App)
	dst.Holiday = mergeHoliday(dst.Holiday, src.Holiday)
	dst.Notification = mergeNotification(dst.Notification, src.Notification)
	dst.Update = mergeUpdate(dst.Update, src.Update)
	dst.RandomDelay = mergeRandomDelay(dst.RandomDelay, src.RandomDelay)
	dst.AppState = mergeAppState(dst.AppState, src.AppState)
	dst.MakeupWindow = mergeMakeupWindow(dst.MakeupWindow, src.MakeupWindow)
	dst.Advanced = mergeAdvanced(dst.Advanced, src.Advanced)

	// Checkin map：逐条目合并，不覆盖用户未提供的条目
	if src.Checkin != nil {
		if dst.Checkin == nil {
			dst.Checkin = make(map[string]CheckinEntry)
		}
		for k, v := range src.Checkin {
			dst.Checkin[k] = v
		}
	}

	return dst
}

func mergeMuMu(dst, src MuMuConfig) MuMuConfig {
	if src.Host != "" {
		dst.Host = src.Host
	}
	if src.Port != 0 {
		dst.Port = src.Port
	}
	if src.AdbPath != "" {
		dst.AdbPath = src.AdbPath
	}
	if src.MuMuExePath != "" {
		dst.MuMuExePath = src.MuMuExePath
	}
	if src.GpsLatitude != 0 {
		dst.GpsLatitude = src.GpsLatitude
	}
	if src.GpsLongitude != 0 {
		dst.GpsLongitude = src.GpsLongitude
	}
	return dst
}

func mergeApp(dst, src AppConfig) AppConfig {
	if src.PackageName != "" {
		dst.PackageName = src.PackageName
	}
	if src.Activity != "" {
		dst.Activity = src.Activity
	}
	return dst
}

func mergeHoliday(dst, src HolidayConfig) HolidayConfig {
	if !src.SkipWeekend {
		dst.SkipWeekend = src.SkipWeekend
	}
	if !src.SkipHoliday {
		dst.SkipHoliday = src.SkipHoliday
	}
	if src.ExtraWorkdays != nil {
		dst.ExtraWorkdays = src.ExtraWorkdays
	}
	if src.ExtraHolidays != nil {
		dst.ExtraHolidays = src.ExtraHolidays
	}
	return dst
}

func mergeNotification(dst, src NotificationConfig) NotificationConfig {
	if src.Enabled {
		dst.Enabled = src.Enabled
	}
	if src.Webhook != "" {
		dst.Webhook = src.Webhook
	}
	if !src.VerifyTLS {
		dst.VerifyTLS = src.VerifyTLS
	}
	return dst
}

func mergeUpdate(dst, src UpdateConfig) UpdateConfig {
	if src.ManifestURL != "" {
		dst.ManifestURL = src.ManifestURL
	}
	if src.AutoCheckOnStartup {
		dst.AutoCheckOnStartup = src.AutoCheckOnStartup
	}
	return dst
}

func mergeRandomDelay(dst, src RandomDelayConfig) RandomDelayConfig {
	if src.MinSeconds != 0 {
		dst.MinSeconds = src.MinSeconds
	}
	if src.MaxSeconds != 0 {
		dst.MaxSeconds = src.MaxSeconds
	}
	return dst
}

func mergeAppState(dst, src AppStateConfig) AppStateConfig {
	if src.AutoConnect {
		dst.AutoConnect = src.AutoConnect
	}
	if src.AutoStart {
		dst.AutoStart = src.AutoStart
	}
	if src.KeepAliveEnabled {
		dst.KeepAliveEnabled = src.KeepAliveEnabled
	}
	if src.RecoveryBaseBackoffSeconds != 0 {
		dst.RecoveryBaseBackoffSeconds = src.RecoveryBaseBackoffSeconds
	}
	if src.RecoveryMaxBackoffSeconds != 0 {
		dst.RecoveryMaxBackoffSeconds = src.RecoveryMaxBackoffSeconds
	}
	if src.RecoveryMaxFailures != 0 {
		dst.RecoveryMaxFailures = src.RecoveryMaxFailures
	}
	if src.RecoveryPauseMinutesAfterMax != 0 {
		dst.RecoveryPauseMinutesAfterMax = src.RecoveryPauseMinutesAfterMax
	}
	if src.RecoveryQuietHoursEnabled {
		dst.RecoveryQuietHoursEnabled = src.RecoveryQuietHoursEnabled
	}
	if src.RecoveryQuietStartHour != 0 {
		dst.RecoveryQuietStartHour = src.RecoveryQuietStartHour
	}
	if src.RecoveryQuietEndHour != 0 {
		dst.RecoveryQuietEndHour = src.RecoveryQuietEndHour
	}
	return dst
}

func mergeMakeupWindow(dst, src MakeupWindowConfig) MakeupWindowConfig {
	if src.MorningSignin != nil {
		dst.MorningSignin = src.MorningSignin
	}
	if src.MorningSignout != nil {
		dst.MorningSignout = src.MorningSignout
	}
	if src.AfternoonSignin != nil {
		dst.AfternoonSignin = src.AfternoonSignin
	}
	if src.AfternoonSignout != nil {
		dst.AfternoonSignout = src.AfternoonSignout
	}
	return dst
}

func mergeAdvanced(dst, src AdvancedConfig) AdvancedConfig {
	if src.SessionTTLSeconds != 0 {
		dst.SessionTTLSeconds = src.SessionTTLSeconds
	}
	if src.MisfireGraceSeconds != 0 {
		dst.MisfireGraceSeconds = src.MisfireGraceSeconds
	}
	if src.NetworkProbe.Targets != nil {
		dst.NetworkProbe.Targets = src.NetworkProbe.Targets
	}
	if src.NetworkProbe.TimeoutSeconds != 0 {
		dst.NetworkProbe.TimeoutSeconds = src.NetworkProbe.TimeoutSeconds
	}
	return dst
}
