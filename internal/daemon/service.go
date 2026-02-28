package daemon

// ServiceManager abstracts platform-specific service management (launchd, systemd, etc.).
type ServiceManager interface {
	// IsInstalled returns true if the service is registered with the platform service manager.
	IsInstalled() bool

	// IsRunning returns whether the managed service is running and its PID.
	IsRunning() (bool, int)

	// Install registers the service with the platform service manager.
	Install(execPath, configPath string) error

	// Uninstall removes the service from the platform service manager.
	Uninstall() error

	// Start starts the service via the platform service manager.
	Start() error

	// Stop stops the service via the platform service manager.
	Stop() error

	// ServiceName returns a human-readable name for the service backend (e.g. "launchd", "systemd").
	ServiceName() string
}
