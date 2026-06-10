package automator

import (
	"math/rand"
	"strings"
	"time"
)

// GenerateRandomTime 在指定时间范围内生成随机时间
func GenerateRandomTime(startTime, endTime string, extraMinSeconds, extraMaxSeconds int) time.Time {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	sh, sm := parseTimeStr(startTime)
	eh, em := parseTimeStr(endTime)

	startMinutes := sh*60 + sm
	endMinutes := eh*60 + em

	var targetMinutes int
	delta := endMinutes - startMinutes
	if delta > 1 {
		targetMinutes = startMinutes + rand.Intn(delta)
	} else {
		targetMinutes = startMinutes
	}

	baseTime := today.Add(time.Duration(targetMinutes) * time.Minute)

	if extraMaxSeconds > 0 && extraMinSeconds >= 0 {
		extra := extraMinSeconds
		extraDelta := extraMaxSeconds - extraMinSeconds
		if extraDelta > 0 {
			extra += rand.Intn(extraDelta + 1)
		}
		baseTime = baseTime.Add(time.Duration(extra) * time.Second)
	}

	return baseTime
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
	s = strings.TrimSpace(s)
	if len(s) < 5 || s[2] != ':' {
		return 0, 0
	}
	h := int(s[0]-'0')*10 + int(s[1]-'0')
	m := int(s[3]-'0')*10 + int(s[4]-'0')
	if h > 23 || m > 59 {
		return 0, 0
	}
	return h, m
}
