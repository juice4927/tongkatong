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

// DefaultConfigJSON 返回默认配置的 JSON 字符串
func DefaultConfigJSON() string {
	data, _ := json.MarshalIndent(defaultConfig(), "", "  ")
	return string(data)
}

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

	// 标准化打卡配置（补全缺失字段、校验降级）
	normalizeCheckin(cfg)

	// 备份损坏的配置文件
	if err := cm.backupIfCorrupted(); err != nil {
		slog.Warn("配置备份失败", "error", err)
	}

	if !userExists {
		_ = cm.writeConfig(cfg)
	}

	return cfg
}

// normalizeCheckin 标准化打卡配置：补全缺失标签、校验时间格式、降级无效值
func normalizeCheckin(cfg *Config) {
	for key, entry := range cfg.Checkin {
		if entry.Label == "" {
			switch key {
			case "morning_signin":
				entry.Label = "上午签到"
			case "morning_signout":
				entry.Label = "上午签退"
			case "afternoon_signin":
				entry.Label = "下午签到"
			case "afternoon_signout":
				entry.Label = "下午签退"
			default:
				entry.Label = key
			}
		}
		if len(entry.TimeRange) != 2 {
			entry.TimeRange = []string{"00:00", "00:00"}
		}
		for i := range entry.TimeRange {
			if !isValidHHMM(entry.TimeRange[i]) {
				entry.TimeRange[i] = "00:00"
			}
		}
		cfg.Checkin[key] = entry
	}
}

// backupIfCorrupted 备份用户配置（.json.bak）防止写入损坏
func (cm *ConfigManager) backupIfCorrupted() error {
	if _, err := os.Stat(cm.configFile); os.IsNotExist(err) {
		return nil
	}
	data, err := os.ReadFile(cm.configFile)
	if err != nil {
		return nil // 读失败，不备份
	}
	// 简单验证：是否有效 JSON
	if json.Valid(data) {
		return nil // 有效，不需要备份
	}
	// 无效 JSON → 备份
	backupPath := cm.configFile + ".bak"
	slog.Warn("检测到损坏的配置文件，创建备份", "backup", backupPath)
	_ = os.WriteFile(backupPath, data, 0644)
	return os.Rename(cm.configFile, cm.configFile+".corrupted")
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

// ValidateConfig 验证配置有效性，返回 (是否有效, 错误消息)
func (cm *ConfigManager) ValidateConfig() (bool, string) {
	cfg := cm.Config()

	if cfg.MuMu.Port <= 0 || cfg.MuMu.Port > 65535 {
		return false, fmt.Sprintf("端口号无效: %d（范围 1-65535）", cfg.MuMu.Port)
	}
	if cfg.MuMu.Host == "" {
		return false, "主机地址不能为空"
	}

	if cfg.App.PackageName == "" {
		return false, "应用包名不能为空"
	}

	// GPS 坐标校验
	if cfg.MuMu.GpsLatitude != 0 || cfg.MuMu.GpsLongitude != 0 {
		if cfg.MuMu.GpsLatitude < -90 || cfg.MuMu.GpsLatitude > 90 {
			return false, fmt.Sprintf("GPS 纬度无效: %.6f（范围 -90 ~ 90）", cfg.MuMu.GpsLatitude)
		}
		if cfg.MuMu.GpsLongitude < -180 || cfg.MuMu.GpsLongitude > 180 {
			return false, fmt.Sprintf("GPS 经度无效: %.6f（范围 -180 ~ 180）", cfg.MuMu.GpsLongitude)
		}
	}

	// 打卡时间校验
	for key, entry := range cfg.Checkin {
		if !entry.Enabled {
			continue
		}
		if len(entry.TimeRange) != 2 {
			return false, fmt.Sprintf("打卡时段 %s 时间范围格式无效", key)
		}
		if !isValidHHMM(entry.TimeRange[0]) || !isValidHHMM(entry.TimeRange[1]) {
			return false, fmt.Sprintf("打卡时段 %s 时间格式无效（需 HH:MM）", key)
		}
	}

	// 补签窗口校验
	for key, window := range map[string][]int{
		"morning_signin":    cfg.MakeupWindow.MorningSignin,
		"morning_signout":   cfg.MakeupWindow.MorningSignout,
		"afternoon_signin":  cfg.MakeupWindow.AfternoonSignin,
		"afternoon_signout": cfg.MakeupWindow.AfternoonSignout,
	} {
		if len(window) != 4 {
			continue
		}
		if window[0] < 0 || window[0] > 48 || window[2] < 0 || window[2] > 48 {
			return false, fmt.Sprintf("补签窗口 %s 小时无效（范围 0-48）", key)
		}
		if window[1] < 0 || window[1] > 59 || window[3] < 0 || window[3] > 59 {
			return false, fmt.Sprintf("补签窗口 %s 分钟无效（范围 0-59）", key)
		}
	}

	// 恢复策略校验
	if cfg.AppState.RecoveryBaseBackoffSeconds > cfg.AppState.RecoveryMaxBackoffSeconds {
		return false, "恢复策略无效：基准退避不能大于最大退避"
	}

	return true, ""
}

func isValidHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}
