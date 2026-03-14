//go:build windows

package daemon

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const winServiceName = "mnemonic"
const winServiceDisplayName = "Mnemonic - Semantic Memory System"
const winServiceDescription = "Local-first semantic memory system with cognitive agents."

type windowsServiceManager struct{}

// NewServiceManager returns the Windows service manager.
func NewServiceManager() ServiceManager {
	return &windowsServiceManager{}
}

func (m *windowsServiceManager) IsInstalled() bool {
	scm, err := mgr.Connect()
	if err != nil {
		return false
	}
	defer scm.Disconnect()

	s, err := scm.OpenService(winServiceName)
	if err != nil {
		return false
	}
	_ = s.Close()
	return true
}

func (m *windowsServiceManager) IsRunning() (bool, int) {
	scm, err := mgr.Connect()
	if err != nil {
		return false, 0
	}
	defer scm.Disconnect()

	s, err := scm.OpenService(winServiceName)
	if err != nil {
		return false, 0
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return false, 0
	}

	if status.State == svc.Running {
		return true, int(status.ProcessId)
	}
	return false, 0
}

func (m *windowsServiceManager) Install(execPath, configPath string) error {
	// Resolve to absolute paths
	var err error
	execPath, err = filepath.Abs(execPath)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolving config path: %w", err)
	}

	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager (run as Administrator): %w", err)
	}
	defer scm.Disconnect()

	s, err := scm.CreateService(
		winServiceName,
		execPath,
		mgr.Config{
			DisplayName:  winServiceDisplayName,
			Description:  winServiceDescription,
			StartType:    mgr.StartAutomatic,
			ErrorControl: mgr.ErrorNormal,
		},
		"--config", configPath, "serve",
	)
	if err != nil {
		return fmt.Errorf("creating service: %w", err)
	}
	defer s.Close()

	// Configure recovery: restart on failure after 5 seconds
	recoveryActions := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.NoAction, Delay: 0},
	}
	// Best-effort: recovery config is optional, ignore errors.
	_ = s.SetRecoveryActions(recoveryActions, 86400)

	return nil
}

func (m *windowsServiceManager) Uninstall() error {
	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager (run as Administrator): %w", err)
	}
	defer scm.Disconnect()

	s, err := scm.OpenService(winServiceName)
	if err != nil {
		return fmt.Errorf("opening service: %w", err)
	}
	defer s.Close()

	// Stop the service if running (ignore errors — may already be stopped)
	_, _ = s.Control(svc.Stop)

	if err := s.Delete(); err != nil {
		return fmt.Errorf("deleting service: %w", err)
	}

	return nil
}

func (m *windowsServiceManager) Start() error {
	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer scm.Disconnect()

	s, err := scm.OpenService(winServiceName)
	if err != nil {
		return fmt.Errorf("opening service: %w", err)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("starting service: %w", err)
	}
	return nil
}

func (m *windowsServiceManager) Stop() error {
	scm, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connecting to service manager: %w", err)
	}
	defer scm.Disconnect()

	s, err := scm.OpenService(winServiceName)
	if err != nil {
		return fmt.Errorf("opening service: %w", err)
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	if err != nil {
		return fmt.Errorf("stopping service: %w", err)
	}
	return nil
}

func (m *windowsServiceManager) Restart() error {
	// Use sc.exe to restart — spawned as a background process so it outlives the current binary.
	return exec.Command("cmd", "/C", "net stop "+winServiceName+" && net start "+winServiceName).Start()
}

func (m *windowsServiceManager) ServiceName() string {
	return "windows-service"
}

// IsWindowsService reports whether the current process is running as a
// Windows Service (invoked by the Service Control Manager).
func IsWindowsService() bool {
	isSvc, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isSvc
}

// RunAsService runs the given command as a Windows Service handler.
// execPath and configPath are used to build the serve command arguments.
func RunAsService(execPath, configPath string) error {
	return svc.Run(winServiceName, &mnemonicService{
		execPath:   execPath,
		configPath: configPath,
	})
}

// mnemonicService implements svc.Handler.
type mnemonicService struct {
	execPath   string
	configPath string
}

// Execute implements the Windows Service handler. It spawns a child "mnemonic serve"
// process rather than running serve logic in-process. The child is not detected as a
// Windows Service (SCM pipe detection fails for child processes), so it runs the full
// daemon lifecycle normally. This keeps SCM management separate from application logic.
// TODO: refactor to run serve logic directly in Execute() to avoid the two-process model.
func (s *mnemonicService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	// Start the serve process as a child (see comment above for rationale).
	cmd := exec.Command(s.execPath, "--config", s.configPath, "serve")
	if err := cmd.Start(); err != nil {
		return true, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: accepted}

	// Wait for stop/shutdown signal from SCM
	for {
		c := <-r
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			changes <- svc.Status{State: svc.StopPending}
			// Kill the child process
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			_ = cmd.Wait()
			return false, 0
		}
	}
}
