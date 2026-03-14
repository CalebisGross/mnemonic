//go:build !darwin && !linux && !windows

package daemon

import "fmt"

type stubManager struct{}

// NewServiceManager returns a stub service manager for unsupported platforms.
func NewServiceManager() ServiceManager {
	return &stubManager{}
}

func (m *stubManager) IsInstalled() bool      { return false }
func (m *stubManager) IsRunning() (bool, int) { return false, 0 }
func (m *stubManager) ServiceName() string    { return "unsupported" }

func (m *stubManager) Install(execPath, configPath string) error {
	return fmt.Errorf("service management is not supported on this platform")
}

func (m *stubManager) Uninstall() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func (m *stubManager) Start() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func (m *stubManager) Stop() error {
	return fmt.Errorf("service management is not supported on this platform")
}

func (m *stubManager) Restart() error {
	return fmt.Errorf("service management is not supported on this platform")
}
