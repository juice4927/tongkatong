//go:build !windows

package autostart

func IsEnabled() bool {
	return false
}

func SetEnabled(enable bool) error {
	return nil
}
