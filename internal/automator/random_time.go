package automator

import (
	"math/rand"
	"time"
)

// GenerateRandomTime 在指定时间范围内生成随机时间
//
//	startTime: "HH:MM"
//	endTime: "HH:MM"
//	extraSeconds: [min, max] 额外随机秒数
func GenerateRandomTime(startTime, endTime string, extraMinSeconds, extraMaxSeconds int) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	sh, sm := parseTimeStr(startTime)
	eh, em := parseTimeStr(endTime)

	startMinutes := sh*60 + sm
	endMinutes := eh*60 + em

	var targetMinutes int
	if endMinutes > startMinutes {
		targetMinutes = startMinutes + rand.Intn(endMinutes-startMinutes)
	} else {
		targetMinutes = startMinutes
	}

	baseTime := today.Add(time.Duration(targetMinutes) * time.Minute)

	// 增加随机秒数
	if extraMaxSeconds > 0 && extraMinSeconds >= 0 {
		extra := extraMinSeconds
		if extraMaxSeconds > extraMinSeconds {
			extra += rand.Intn(extraMaxSeconds - extraMinSeconds + 1)
		}
		baseTime = baseTime.Add(time.Duration(extra) * time.Second)
	}

	return baseTime
}

// GenerateCheckinTimes 生成当日所有打卡时段的随机时间
func GenerateCheckinTimes(checkinTimes map[string]struct {
	Enabled   bool
	TimeRange []string
	Label     string
}, delayMin, delayMax int) map[string]time.Time {
	results := make(map[string]time.Time)

	for key, ct := range checkinTimes {
		if !ct.Enabled || len(ct.TimeRange) != 2 {
			continue
		}
		if ct.TimeRange[0] == ct.TimeRange[1] || ct.TimeRange[0] == "00:00" {
			continue
		}
		results[key] = GenerateRandomTime(ct.TimeRange[0], ct.TimeRange[1], delayMin, delayMax)
	}

	return results
}

// NowHHMM 获取当前时间的 "HH:MM" 字符串
func NowHHMM() string {
	now := time.Now()
	return formatInt(now.Hour()) + ":" + formatInt(now.Minute())
}

// FormatTimeForDisplay 格式化时间用于显示
func FormatTimeForDisplay(t time.Time) string {
	if t.IsZero() {
		return "--:--"
	}
	return t.Format("15:04")
}

// parseTimeStr 解析 "HH:MM" → (hour, minute)
func parseTimeStr(s string) (int, int) {
	if len(s) < 5 || s[2] != ':' {
		return 0, 0
	}
	return int(s[0]-'0')*10 + int(s[1]-'0'), int(s[3]-'0')*10 + int(s[4]-'0')
}

func formatInt(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
