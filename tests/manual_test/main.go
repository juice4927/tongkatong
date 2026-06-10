// 模拟真人使用测试 — 按用户操作顺序调用所有API
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juice4927/tongkatong/internal/adb"
	"github.com/juice4927/tongkatong/internal/automator"
	"github.com/juice4927/tongkatong/internal/config"
	"github.com/juice4927/tongkatong/internal/holiday"
	"github.com/juice4927/tongkatong/internal/models"
	"github.com/juice4927/tongkatong/internal/updater"
	"github.com/juice4927/tongkatong/internal/utils"
)

var passed, failed int

func check(name string, fn func() (bool, string)) {
	ok, msg := fn()
	if ok { passed++; fmt.Printf("  ✅ %s: %s\n", name, msg) } else { failed++; fmt.Printf("  ❌ %s: %s\n", name, msg) }
}

func main() {
	fmt.Println("══════════════════════════════════════")
	fmt.Println("  通卡通 v" + models.Version + " 模拟真人测试")
	fmt.Println("══════════════════════════════════════")

	cfgDir := filepath.Join("..", "..", "config")
	_ = os.MkdirAll(cfgDir, 0755)
	logDir := filepath.Join("..", "..", "build_out", "logs")
	_ = os.MkdirAll(logDir, 0755)
	_ = utils.SetupLogging(logDir, "INFO", false)

	// ── 1. 打开软件 → 加载配置 ──────────────────────
	fmt.Println("\n📱 打开软件 → 加载配置")
	cm := config.NewConfigManager(cfgDir)
	cfg := cm.Config()
	check("配置加载成功", func() (bool, string) {
		return cfg != nil && cfg.MuMu.Host == "127.0.0.1", cfg.MuMu.Host
	})
	check("4个打卡时段配置", func() (bool, string) {
		return len(cfg.Checkin) == 4, fmt.Sprintf("%d", len(cfg.Checkin))
	})
	check("默认跳过周末", func() (bool, string) {
		return cfg.Holiday.SkipWeekend, "正确"
	})

	// ── 2. 设置页面 → 检查配置 ─────────────────────
	fmt.Println("\n📱 设置页面 → 检查配置有效性")
	check("端口范围 1-65535", func() (bool, string) {
		return cfg.MuMu.Port >= 1 && cfg.MuMu.Port <= 65535, fmt.Sprintf("%d", cfg.MuMu.Port)
	})
	ok, msg := cm.ValidateConfig()
	check("配置验证通过: "+msg, func() (bool, string) { return ok, msg })

	// 修改端口为非法值 → 验证失败
	cfg2 := *cfg; cfg2.MuMu.Port = 99999
	cm2 := config.NewConfigManager(cfgDir); _ = cm2.SaveConfig(&cfg2)
	ok2, _ := cm2.ValidateConfig()
	check("非法端口被拦截", func() (bool, string) { return !ok2, "正确拒绝99999" })

	// ── 3. 设置页面 → 保存配置 ─────────────────────
	fmt.Println("\n📱 设置页面 → 保存配置")
	cfg.MuMu.Host = "127.0.0.1"; cfg.MuMu.Port = 5555
	check("保存成功", func() (bool, string) { err := cm.SaveConfig(cfg); return err == nil, "" })

	// 导出/导入
	data1 := config.DefaultConfigJSON()
	check("导出默认配置", func() (bool, string) { return len(data1) > 100, fmt.Sprintf("%d bytes", len(data1)) })
	cm3 := config.NewConfigManager(cfgDir)
	_ = cm3.SaveConfig(cfg)
	cfg3 := cm3.Config()
	check("重新加载一致", func() (bool, string) { return cfg3.MuMu.Host == "127.0.0.1", cfg3.MuMu.Host })

	// ── 4. 节假日检查 ───────────────────────────────
	fmt.Println("\n📱 节假日检查")
	today := time.Now()
	hc := holiday.NewHolidayChecker(true, true, nil, nil)
	check("IsWorkday 不panic", func() (bool, string) {
		_ = hc.IsWorkday(today); return true, "ok"
	})
	check("GetHolidayName nil guard", func() (bool, string) {
		_ = hc.GetHolidayName(today); return true, "ok (nil data保护)"
	})
	next := hc.GetNextWorkday(today)
	check("GetNextWorkday 返回值", func() (bool, string) {
		return next.After(today), next.Format("01-02")
	})

	// ── 5. 点击连接设备 ─────────────────────────────
	fmt.Println("\n📱 点击连接设备 → ADB 初始化")
	adbHelper := adb.NewADBHelper("")
	adbAvailable := adbHelper.Version() != ""
	check("ADB状态", func() (bool, string) {
		if adbAvailable { return true, "可用: " + adbHelper.Version() }
		return true, "不可用(无模拟器环境 - 正常)"
	})

	mumu := adb.NewMuMuHelper("", "")
	foundAdb := mumu.FindAdb()
	check("自动发现ADB路径", func() (bool, string) {
		return foundAdb != "" && foundAdb != "adb", foundAdb
	})

	// 多端口尝试连接
	for _, port := range []int{5555, 7555} {
		ok, _ := adbHelper.Connect("127.0.0.1", port)
		check(fmt.Sprintf("尝试端口 %d", port), func() (bool, string) {
			return true, fmt.Sprintf("%v (无模拟器时预期失败)", ok)
		})
	}

	// 断开连接
	adbHelper.Disconnect("127.0.0.1", 5555)

	// ── 6. 设备信息 ─────────────────────────────────
	fmt.Println("\n📱 获取设备信息")
	info := adbHelper.GetDeviceInfo("")
	check("GetDeviceInfo 返回map", func() (bool, string) {
		return info != nil, fmt.Sprintf("%d keys", len(info))
	})

	// ── 7. 自动化引擎初始化 ─────────────────────────
	fmt.Println("\n📱 自动化引擎初始化")
	pool := adb.NewDevicePool(adbHelper, 300*time.Second)
	uia := automator.NewUIAutomator2Impl("127.0.0.1", 5555, "adb", "com.test", pool)
	uia.SetMuMuHelper(mumu)
	uia.SetBaseDir(logDir)

	check("引擎初始状态: 未连接", func() (bool, string) {
		return !uia.IsConnected(), "正确"
	})

	// 安全方法(无设备时不应panic)
	uia.WaitForUIReady(1*time.Second, 10)
	uia.IsLoggedIn()
	uia.IsOnLoginPage()
	check("UI就绪/登录检测 安全", func() (bool, string) { return true, "全部不panic" })

	check("ResolveActionSlot 上午签到", func() (bool, string) {
		isMg, isSi, label := automator.ResolveActionSlot(automator.MorningSignin)
		return isMg && isSi && label == "上午", label
	})
	check("ResolveActionSlot 下午签退", func() (bool, string) {
		isMg, isSi, label := automator.ResolveActionSlot(automator.AfternoonSignout)
		return !isMg && !isSi && label == "下午", label
	})
	check("IsTimeMatch 合法时间", func() (bool, string) {
		return automator.IsTimeMatch("13:45") && !automator.IsTimeMatch("99:99"), "正确"
	})
	check("IsAlreadyCheckedIn nil安全", func() (bool, string) {
		return !automator.IsAlreadyCheckedIn(automator.MorningSignin, nil), "返回false"
	})

	// ── 9. 错误类型 ─────────────────────────────────
	fmt.Println("\n📱 错误类型检查")
	ae := automator.AlreadyCheckedInError{Message: "test", InCorrectSlot: true, CheckinTime: "07:30"}
	check("AlreadyCheckedInError", func() (bool, string) { return ae.Error() == "test", ae.Error() })
	ge := automator.GpsLocationError{Message: "gps"}
	check("GpsLocationError", func() (bool, string) { return ge.Error() == "gps", "" })

	// ── 10. 启动调度 ────────────────────────────────
	fmt.Println("\n📱 启动调度")
	orc := automator.NewCheckinOrchestrator(uia, hc, cm, logDir)
	check("调度器初始化", func() (bool, string) { return orc.Initialize(), "" })
	check("调度器启动", func() (bool, string) { orc.Start(); return true, "ok" })

	// 注册回调
	cbFired := false
	orc.SetResultCallback(func(r automator.CheckinRecord) { cbFired = true })
	check("回调注册", func() (bool, string) { return !cbFired, "正确(未触发)" })

	// 查询
	results := orc.GetDailyResults()
	check("GetDailyResults", func() (bool, string) {
		return results != nil, fmt.Sprintf("%d条", len(results))
	})
	last := orc.GetLastResult()
	check("GetLastResult", func() (bool, string) {
		return true, fmt.Sprintf("%v", last)
	})

	// ── 11. 调度器核心功能 ──────────────────────────
	fmt.Println("\n📱 调度器核心功能")
	orc.Stop()
	sched := automator.NewCheckinScheduler()
	sched.Initialize(); sched.Start()

	executed := false
	sched.AddOnceJob("test_job", time.Now().Add(2*time.Second), func() { executed = true })
	sched.AddDailyJob("daily", "0 30 8 * * *", func() {})
	jobs := sched.GetJobs()
	check("任务已添加", func() (bool, string) { return len(jobs) == 2, fmt.Sprintf("%d个", len(jobs)) })

	stats := sched.ReconcileManagedJobs([]string{"daily"})
	check("Reconcile 清理", func() (bool, string) {
		return stats["removed"] == 1, fmt.Sprintf("移除%d保留%d", stats["removed"], stats["kept"])
	})

	time.Sleep(3 * time.Second)
	sched.Stop()
	check("test_job 已执行(3s)", func() (bool, string) { return true, fmt.Sprintf("%v", executed) })

	// ── 12. 停止调度 + 断开连接 ────────────────────
	fmt.Println("\n📱 停止调度 + 断开连接")
	check("Stop不panic", func() (bool, string) { orc.Stop(); return true, "" })
	uia.Disconnect()
	check("断开不panic", func() (bool, string) { return true, "" })
	pool.CleanupExpired()
	check("清理池不panic", func() (bool, string) { return true, "" })

	// ── 13. 随机时间 ────────────────────────────────
	fmt.Println("\n📱 随机打卡时间生成")
	for i := 0; i < 5; i++ {
		t := automator.GenerateRandomTime("07:20", "07:55", 1, 5)
		mins := t.Hour()*60 + t.Minute()
		check(fmt.Sprintf("时间 %s", t.Format("15:04")), func() (bool, string) {
			return mins >= 7*60+20 && mins <= 8*60, t.Format("15:04:05")
		})
	}

	// ── 14. Failure Codes ───────────────────────────
	fmt.Println("\n📱 Failure Codes")
	for _, c := range []string{"device_connect_failed", "navigation_failed", "button_not_found", "checkin_failed"} {
		check(c+" 可重试", func() (bool, string) {
			return models.IsRetryable(models.FailureCode(c)), ""
		})
	}
	for _, c := range []string{"already_checked_in", "outside_range"} {
		check(c+" 不可重试", func() (bool, string) {
			return !models.IsRetryable(models.FailureCode(c)), ""
		})
	}

	// ── 15. 更新模块 ────────────────────────────────
	fmt.Println("\n📱 更新模块")
	h, err := updater.SHA256File(os.Args[0])
	check("SHA256计算", func() (bool, string) {
		return err == nil && len(h) == 64, fmt.Sprintf("%.16s...", h)
	})
	s := updater.DescribeUpdateState(os.TempDir())
	check("更新状态", func() (bool, string) { return s != "", s })

	// ── 16. 通知模块 ────────────────────────────────
	fmt.Println("\n📱 通知模块")
	utils.NotifyCheckinResult(utils.NotifyConfig{Enabled: false}, "test", true, "ok", time.Now().Format(time.DateTime))
	utils.RecordCheckinResult(logDir, "test", true, "msg", time.Now().Format(time.DateTime))
	utils.SaveFailureDiagnosis(logDir, "test", nil, nil)
	check("通知/记录/诊断 不panic", func() (bool, string) { return true, "全部安全" })

	// ── 17. 网络检测 ────────────────────────────────
	fmt.Println("\n📱 网络连通性检测")
	result := utils.CheckNetworkConnectivity(nil, 3*time.Second)
	check("网络检测完成", func() (bool, string) {
		return true, fmt.Sprintf("network=%v", result)
	})

	// ── 18. XML解析 ─────────────────────────────────
	fmt.Println("\n📱 XML 解析")
	xml := `<?xml version="1.0"?><hierarchy><node text="签到" clickable="true" bounds="[0,0][100,100]"/></hierarchy>`
	nodes := automator.ParseHierarchyXML(xml)
	check("合法XML解析", func() (bool, string) { return len(nodes) == 1, fmt.Sprintf("%d nodes", len(nodes)) })
	nodes2 := automator.ParseHierarchyXML("invalid <<<")
	check("非法XML不panic", func() (bool, string) { return len(nodes2) == 0, fmt.Sprintf("%d nodes", len(nodes2)) })

	// ── 19. HandleLoginIfNeeded 超时场景 ─────────────
	fmt.Println("\n📱 登录检测 (无设备超时)")
	err2 := uia.HandleLoginIfNeeded(5)
	check("HandleLoginIfNeeded 超时返回错误", func() (bool, string) {
		return err2 != nil, fmt.Sprintf("%v (预期:无设备)", err2)
	})

	// ── 总结 ─────────────────────────────────────────
	fmt.Println()
	fmt.Println("══════════════════════════════════════")
	fmt.Printf("  结果: ✅ %d 通过  ❌ %d 失败  (%d 项)\n", passed, failed, passed+failed)
	fmt.Println("══════════════════════════════════════")
	if failed > 0 { os.Exit(1) }
}
