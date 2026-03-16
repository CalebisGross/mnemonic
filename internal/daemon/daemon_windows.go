//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows"
)

// IsRunning checks if the daemon process is running.
// Returns (isRunning, pid).
func IsRunning() (bool, int) {
	pid, err := ReadPID()
	if err != nil {
		return false, 0
	}

	// On Windows, os.FindProcess always succeeds even for dead processes.
	// Open the process handle to verify it actually exists.
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, 0
	}
	_ = windows.CloseHandle(handle)

	return true, pid
}

// Start launches the daemon process with the given exec path and config path.
// It creates a detached background process and writes the PID.
func Start(execPath string, configPath string) (int, error) {
	cmd := exec.Command(execPath, "--config", configPath, "serve")

	// Open or create log file for appending
	logPath := LogPath()
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, fmt.Errorf("creating daemon directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("opening log file: %w", err)
	}
	defer logFile.Close()

	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Detach from the current console so the process survives after
	// the launching terminal closes.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting daemon: %w", err)
	}

	pid := cmd.Process.Pid

	if err := WritePID(pid); err != nil {
		return 0, fmt.Errorf("writing PID file: %w", err)
	}

	return pid, nil
}

// PIDRestart spawns a detached background process that waits for the current
// daemon to exit, then starts the new binary. This is the fallback restart
// mechanism when the daemon is not running as a Windows Service.
func PIDRestart(execPath, configPath string) error {
	// Wait 3 seconds for the old process to die, then start the new one.
	script := fmt.Sprintf(
		`timeout /t 3 /nobreak >nul && "%s" --config "%s" start`,
		execPath, configPath,
	)
	cmd := exec.Command("cmd", "/C", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawning restart script: %w", err)
	}
	return nil
}

// Stop stops the daemon process.
// Windows does not support SIGTERM, so we terminate the process directly.
func Stop() error {
	pid, err := ReadPID()
	if err != nil {
		return fmt.Errorf("reading PID: %w", err)
	}

	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// Process is already gone
		_ = RemovePID()
		return nil
	}
	defer windows.CloseHandle(handle)

	// TerminateProcess is a hard kill (no graceful shutdown). GenerateConsoleCtrlEvent
	// is not viable here because the daemon runs in a separate process group
	// (CREATE_NEW_PROCESS_GROUP), and ctrl events cannot cross process groups.
	if err := windows.TerminateProcess(handle, 1); err != nil {
		_ = RemovePID()
		return fmt.Errorf("terminating process: %w", err)
	}

	// Wait up to 5 seconds for the process to exit
	event, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil || event == uint32(windows.WAIT_TIMEOUT) {
		// Best-effort: process may still be exiting
	}

	_ = RemovePID()
	return nil
}
