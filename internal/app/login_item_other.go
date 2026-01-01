//go:build !darwin

package app

import "fmt"

func setLaunchAtLogin(enabled bool) error {
	return fmt.Errorf("launch at login is only supported on macOS")
}
