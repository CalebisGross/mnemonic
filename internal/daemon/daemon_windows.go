//go:build windows

package daemon

import "fmt"

// IsRunning is not yet implemented on Windows.
func IsRunning() (bool, int) {
	return false, 0
}

// Start is not yet implemented on Windows.
func Start(execPath string, configPath string) (int, error) {
	return 0, fmt.Errorf("daemon start is not supported on Windows")
}

// Stop is not yet implemented on Windows.
func Stop() error {
	return fmt.Errorf("daemon stop is not supported on Windows")
}
