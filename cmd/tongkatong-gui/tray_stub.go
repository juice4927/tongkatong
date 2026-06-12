//go:build !windows

package main

type trayController interface {
	Start()
	Stop()
	Refresh()
	Enabled() bool
}

type noopTray struct{}

func newTrayController(app *App) trayController {
	return noopTray{}
}

func (noopTray) Start()        {}
func (noopTray) Stop()         {}
func (noopTray) Refresh()      {}
func (noopTray) Enabled() bool { return false }
