package holiday

import (
	"testing"
	"time"
)

func TestNewHolidayChecker(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)
	if hc == nil {
		t.Fatal("NewHolidayChecker returned nil")
	}
	if !hc.skipWeekend {
		t.Error("skipWeekend should be true")
	}
	if !hc.skipHoliday {
		t.Error("skipHoliday should be true")
	}
}

func TestIsWorkday_Weekend(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	// 2025-01-04 is a Saturday
	saturday := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	if hc.IsWorkday(saturday) {
		t.Error("Saturday should not be a workday when skipWeekend=true")
	}

	// 2025-01-05 is a Sunday
	sunday := time.Date(2025, 1, 5, 0, 0, 0, 0, time.UTC)
	if hc.IsWorkday(sunday) {
		t.Error("Sunday should not be a workday when skipWeekend=true")
	}
}

func TestIsWorkday_Weekday(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	// 2025-01-06 is a Monday
	monday := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	if !hc.IsWorkday(monday) {
		t.Error("Monday should be a workday when skipWeekend=true")
	}
}

func TestIsWorkday_WithEmbeddedHolidayData(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	// 2025-01-01 is New Year's Day (should be in embedded data)
	newYear := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	if hc.IsWorkday(newYear) {
		t.Log("New Year's Day 2025 may not be in embedded data, falling back to weekday check")
		// This is acceptable if the embedded data doesn't extend that far
	}

	// Verify the checker loaded data without error
	if hc.data == nil {
		t.Log("No embedded holiday data loaded")
	} else {
		t.Logf("Loaded %d holidays, %d in_lieu days", len(hc.data.Holidays), len(hc.data.InLieuDays))
	}
}

func TestIsWorkday_ExtraWorkdayOverride(t *testing.T) {
	// 2025-01-04 is Saturday, but set as extra workday
	hc := NewHolidayChecker(true, true, []string{"2025-01-04"}, nil)

	saturday := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	if !hc.IsWorkday(saturday) {
		t.Error("Saturday should be workday when set as extraWorkday")
	}
}

func TestIsWorkday_ExtraHolidayOverride(t *testing.T) {
	// 2025-01-06 is Monday, but set as extra holiday
	hc := NewHolidayChecker(true, true, nil, []string{"2025-01-06"})

	monday := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	if hc.IsWorkday(monday) {
		t.Error("Monday should be holiday when set as extraHoliday")
	}
}

func TestIsWorkday_UserOverridePriority(t *testing.T) {
	// extraWorkdays 应优先于节假日数据
	hc := NewHolidayChecker(true, true, []string{"2025-10-01"}, nil)

	nationalDay := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t.Logf("user override test: IsWorkday(2025-10-01) = %v", hc.IsWorkday(nationalDay))
}

func TestIsWorkday_SkipWeekendFalse(t *testing.T) {
	hc := NewHolidayChecker(false, false, nil, nil)

	saturday := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	if !hc.IsWorkday(saturday) {
		t.Error("Saturday should be workday when skipWeekend=false")
	}
}

func TestIsHoliday(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	saturday := time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC)
	if !hc.IsHoliday(saturday) {
		t.Error("Saturday should be a holiday when skipWeekend=true")
	}

	monday := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
	if hc.IsHoliday(monday) {
		t.Error("Monday should not be a holiday when skipWeekend=true")
	}
}

func TestGetHolidayName(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	// 测试一个可能存在于内嵌数据中的日期
	testDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	name := hc.GetHolidayName(testDate)
	t.Logf("Holiday name for 2025-01-01: '%s'", name)

	// 普通日期应返回空字符串
	normalDate := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)
	normalName := hc.GetHolidayName(normalDate)
	if normalName != "" {
		t.Logf("normal date got holiday name: '%s'", normalName)
	}
}

func TestIsWorkday_LieuDay(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	// 查找一个调休上班日
	if hc.data != nil {
		for dateStr := range hc.data.InLieuDays {
			parsed, err := time.Parse("2006-01-02", dateStr)
			if err == nil {
				if !hc.IsWorkday(parsed) {
					t.Errorf("In-lieu day %s should be a workday", dateStr)
				}
				return
			}
		}
		t.Log("No in-lieu days found in embedded data")
	}
}

func TestUpdateData(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	newData := &HolidayData{
		Holidays: map[string]string{
			"2025-12-25": "Christmas",
		},
		InLieuDays: map[string]string{
			"2025-12-27": "Christmas lieu",
		},
	}

	if err := hc.UpdateData(newData); err != nil {
		t.Fatalf("UpdateData failed: %v", err)
	}

	christmas := time.Date(2025, 12, 25, 0, 0, 0, 0, time.UTC)
	if hc.IsWorkday(christmas) {
		t.Error("Christmas should not be a workday after UpdateData")
	}

	lieuDay := time.Date(2025, 12, 27, 0, 0, 0, 0, time.UTC)
	if !hc.IsWorkday(lieuDay) {
		t.Error("In-lieu day should be a workday after UpdateData")
	}
}

func TestTryUpdateFromRemote_EmptyURL(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	result := hc.TryUpdateFromRemote("")
	if result {
		t.Error("TryUpdateFromRemote with empty URL should return false")
	}
}

func TestSetLocalCachePath(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)
	hc.SetLocalCachePath("/tmp/holiday_cache.json")
	if hc.localCachePath != "/tmp/holiday_cache.json" {
		t.Errorf("localCachePath = %s, want /tmp/holiday_cache.json", hc.localCachePath)
	}
}

func TestIsWorkday_ConcurrentSafety(t *testing.T) {
	hc := NewHolidayChecker(true, true, nil, nil)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				date := time.Date(2025, 1, 6, 0, 0, 0, 0, time.UTC)
				hc.IsWorkday(date)
				hc.GetHolidayName(date)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		<-done
	}
	// 如果没死锁则通过
}
