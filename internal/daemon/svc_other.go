//go:build !windows

package daemon

// IsWindowsService always returns false on non-Windows platforms.
func IsWindowsService() bool {
	return false
}

// RunAsService is a no-op on non-Windows platforms.
func RunAsService(execPath, configPath string) error {
	return nil
}
