//go:build windows

package main

import (
	"sync"

	"github.com/getlantern/systray"
)

type trayController interface {
	Start()
	Stop()
	Refresh()
	Enabled() bool
}

type guiTray struct {
	app         *App
	startOnce   sync.Once
	stopOnce    sync.Once
	readyCh     chan struct{}
	ready       bool
	showItem    *systray.MenuItem
	hideItem    *systray.MenuItem
	startItem   *systray.MenuItem
	stopItem    *systray.MenuItem
	quitItem    *systray.MenuItem
	statusItem  *systray.MenuItem
	tooltipBase string
}

func newTrayController(app *App) trayController {
	return &guiTray{
		app:         app,
		readyCh:     make(chan struct{}),
		tooltipBase: "通卡通",
	}
}

func (t *guiTray) Enabled() bool {
	return t != nil
}

func (t *guiTray) Start() {
	t.startOnce.Do(func() {
		go systray.Run(t.onReady, t.onExit)
	})
}

func (t *guiTray) Stop() {
	t.stopOnce.Do(func() {
		systray.Quit()
	})
}

func (t *guiTray) Refresh() {
	if !t.ready {
		return
	}

	connected, running := t.app.trayStatus()
	status := "未连接"
	if connected {
		status = "已连接"
	}
	if running {
		status += " · 运行中"
	} else {
		status += " · 已停止"
	}

	if t.statusItem != nil {
		t.statusItem.SetTitle("状态：" + status)
		t.statusItem.Disable()
	}
	systray.SetTooltip(t.tooltipBase + " - " + status)

	if t.startItem != nil {
		if connected && !running {
			t.startItem.Enable()
		} else {
			t.startItem.Disable()
		}
	}
	if t.stopItem != nil {
		if running {
			t.stopItem.Enable()
		} else {
			t.stopItem.Disable()
		}
	}
}

func (t *guiTray) onReady() {
	systray.SetIcon(trayIconICO)
	systray.SetTitle("通卡通")
	systray.SetTooltip(t.tooltipBase)

	t.statusItem = systray.AddMenuItem("状态：初始化中", "当前运行状态")
	t.statusItem.Disable()
	systray.AddSeparator()
	t.showItem = systray.AddMenuItem("显示窗口", "显示主窗口")
	t.hideItem = systray.AddMenuItem("隐藏窗口", "隐藏主窗口")
	systray.AddSeparator()
	t.startItem = systray.AddMenuItem("启动调度", "启动自动打卡调度")
	t.stopItem = systray.AddMenuItem("停止调度", "停止自动打卡调度")
	systray.AddSeparator()
	t.quitItem = systray.AddMenuItem("退出程序", "退出通卡通")

	t.ready = true
	close(t.readyCh)
	t.Refresh()

	go func() {
		for {
			select {
			case <-t.showItem.ClickedCh:
				t.app.ShowWindow()
			case <-t.hideItem.ClickedCh:
				t.app.HideWindow()
			case <-t.startItem.ClickedCh:
				t.app.StartScheduler()
			case <-t.stopItem.ClickedCh:
				t.app.StopScheduler()
			case <-t.quitItem.ClickedCh:
				t.app.Quit()
				return
			}
		}
	}()
}

func (t *guiTray) onExit() {}

var trayIconICO = []byte{
	0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
	0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x20, 0x00, 0x30, 0x00,
	0x00, 0x00, 0x16, 0x00, 0x00, 0x00,
	0x28, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x00, 0x00,
	0x01, 0x00,
	0x20, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x04, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,
	0xBD, 0x6C, 0x0F, 0xFF,
	0x00, 0x00, 0x00, 0x00,
}
