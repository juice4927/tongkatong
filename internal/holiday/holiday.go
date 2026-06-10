// Package holiday 提供节假日判断
package holiday

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ── 调休数据结构 ──────────────────────────────────────────────────

// HolidayData 节假日数据（对应 constants.py 的 holidays + in_lieu_days）
type HolidayData struct {
	Holidays    map[string]string `json:"holidays"`     // date → holiday_name
	InLieuDays map[string]string `json:"in_lieu_days"` // date → holiday_name (调休上班日)
}

//go:embed holidays.json
var embeddedHolidayJSON []byte

// ── HolidayChecker ──────────────────────────────────────────────────

// HolidayChecker 节假日判断模块
type HolidayChecker struct {
	mu            sync.RWMutex
	data          *HolidayData
	localCachePath string

	// 用户配置
	skipWeekend   bool
	skipHoliday   bool
	extraWorkdays map[string]bool
	extraHolidays map[string]bool
}

// NewHolidayChecker 创建节假日检查器
func NewHolidayChecker(skipWeekend, skipHoliday bool, extraWorkdays, extraHolidays []string) *HolidayChecker {
	hc := &HolidayChecker{
		skipWeekend:   skipWeekend,
		skipHoliday:   skipHoliday,
		extraWorkdays: toSet(extraWorkdays),
		extraHolidays: toSet(extraHolidays),
	}

	// 加载数据（优先本地缓存，回退内嵌）
	if err := hc.loadData(); err != nil {
		slog.Warn("节假日数据加载失败，仅使用周末判断", "error", err)
	}
	return hc
}

// SetLocalCachePath 设置本地缓存路径
func (hc *HolidayChecker) SetLocalCachePath(path string) {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.localCachePath = path
}

// TryUpdateFromRemote 尝试从远程更新节假日数据
//
//	下载远程 JSON 并更新本地数据 + 缓存。成功返回 true。
func (hc *HolidayChecker) TryUpdateFromRemote(url string) bool {
	if url == "" {
		return false
	}

	slog.Info("检查节假日数据更新", "url", url)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		slog.Warn("节假日数据远程获取失败", "error", err)
		return false
	}
	defer resp.Body.Close()

	var remoteData HolidayData
	if err := json.NewDecoder(resp.Body).Decode(&remoteData); err != nil {
		slog.Warn("节假日数据远程解析失败", "error", err)
		return false
	}

	if len(remoteData.Holidays) == 0 {
		slog.Warn("远程节假日数据为空，跳过更新")
		return false
	}

	if err := hc.UpdateData(&remoteData); err != nil {
		slog.Warn("节假日数据更新失败", "error", err)
		return false
	}

	slog.Info("节假日数据远程更新成功", "holidays", len(remoteData.Holidays), "in_lieu", len(remoteData.InLieuDays))
	return true
}

// UpdateData 更新节假日数据
func (hc *HolidayChecker) UpdateData(data *HolidayData) error {
	hc.mu.Lock()
	defer hc.mu.Unlock()
	hc.data = data

	// 写本地缓存
	if hc.localCachePath != "" {
		_ = os.MkdirAll(filepath.Dir(hc.localCachePath), 0755)
		jsonData, _ := json.MarshalIndent(data, "", "  ")
		if err := os.WriteFile(hc.localCachePath, jsonData, 0644); err != nil {
			slog.Warn("写入节假日缓存失败", "error", err)
		}
	}
	return nil
}

// IsWorkday 判断是否为工作日
func (hc *HolidayChecker) IsWorkday(checkDate time.Time) bool {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	dateStr := checkDate.Format("2006-01-02")

	// 用户配置最优先
	if hc.extraWorkdays[dateStr] {
		return true
	}
	if hc.extraHolidays[dateStr] {
		return false
	}

	// 从节假日数据判断
	if hc.data != nil {
		// 在 holidays 表中 → 法定节假日 → 非工作日
		if _, isHoliday := hc.data.Holidays[dateStr]; isHoliday {
			return false
		}
		// 在 in_lieu_days 表中 → 调休上班日 → 工作日
		if _, isInLieu := hc.data.InLieuDays[dateStr]; isInLieu {
			return true
		}
	}

	// 降级：纯周末判断
	if hc.skipWeekend {
		weekday := checkDate.Weekday()
		return weekday != time.Saturday && weekday != time.Sunday
	}

	return true
}

// IsHoliday 判断是否为休息日
func (hc *HolidayChecker) IsHoliday(checkDate time.Time) bool {
	return !hc.IsWorkday(checkDate)
}

// GetHolidayName 获取节假日名称
func (hc *HolidayChecker) GetHolidayName(checkDate time.Time) string {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	dateStr := checkDate.Format("2006-01-02")

	if name, ok := hc.data.Holidays[dateStr]; ok {
		return name
	}
	if name, ok := hc.data.InLieuDays[dateStr]; ok {
		return "调休: " + name
	}
	return ""
}

// ── 内部 ───────────────────────────────────────────────────────────

func (hc *HolidayChecker) loadData() error {
	// 尝试从内嵌数据加载
	if len(embeddedHolidayJSON) > 0 {
		var data HolidayData
		if err := json.Unmarshal(embeddedHolidayJSON, &data); err != nil {
			return fmt.Errorf("内嵌节假日数据解析失败: %w", err)
		}
		hc.data = &data
		slog.Info("节假日内嵌数据加载成功")
		return nil
	}

	return fmt.Errorf("内嵌节假日数据为空")
}

func toSet(slice []string) map[string]bool {
	set := make(map[string]bool)
	for _, s := range slice {
		set[s] = true
	}
	return set
}
