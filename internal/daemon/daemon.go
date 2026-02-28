package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

const (
	pidFileName = "mnemonic.pid"
	logFileName = "mnemonic.log"
	mnemDir     = ".mnemonic"
)

// PIDFilePath returns the full path to the PID file.
func PIDFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, mnemDir, pidFileName)
}

// LogPath returns the full path to the log file.
func LogPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, mnemDir, logFileName)
}

// WritePID writes the given PID to the PID file.
func WritePID(pid int) error {
	pidPath := PIDFilePath()
	dir := filepath.Dir(pidPath)

	// Ensure directory exists
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create daemon directory: %w", err)
	}

	// Write PID to file
	content := strconv.Itoa(pid)
	if err := os.WriteFile(pidPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write PID file: %w", err)
	}

	return nil
}

// ReadPID reads the PID from the PID file.
func ReadPID() (int, error) {
	pidPath := PIDFilePath()
	content, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, fmt.Errorf("failed to read PID file: %w", err)
	}

	pid, err := strconv.Atoi(string(content))
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}

	return pid, nil
}

// RemovePID removes the PID file.
func RemovePID() error {
	pidPath := PIDFilePath()
	if err := os.Remove(pidPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove PID file: %w", err)
	}
	return nil
}

// IsRunning checks if the daemon process is running.
// Returns (isRunning, pid).
func IsRunning() (bool, int) {
	pid, err := ReadPID()
	if err != nil {
		return false, 0
	}

	// Check if process exists by sending signal 0
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, 0
	}

	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false, 0
	}

	return true, pid
}

// Start launches the daemon process with the given exec path and config path.
// It forks the process, redirects stdout/stderr to the log file, and writes the PID.
func Start(execPath string, configPath string) (int, error) {
	// Create command
	cmd := exec.Command(execPath, "--config", configPath, "serve")

	// Open or create log file for appending
	logPath := LogPath()
	dir := filepath.Dir(logPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, fmt.Errorf("failed to create daemon directory: %w", err)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	// Redirect stdout and stderr to log file
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Set up process attributes to detach from terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start daemon: %w", err)
	}

	pid := cmd.Process.Pid

	// Write PID file
	if err := WritePID(pid); err != nil {
		return 0, fmt.Errorf("failed to write PID file: %w", err)
	}

	return pid, nil
}

// ============================================================================
// PID-file-based daemon management
// ============================================================================

// Stop stops the daemon process by sending SIGTERM and waiting for it to exit.
// If it doesn't exit within 5 seconds, sends SIGKILL.
func Stop() error {
	pid, err := ReadPID()
	if err != nil {
		return fmt.Errorf("failed to read PID: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	// Send SIGTERM
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	// Wait up to 5 seconds for process to exit
	timeout := time.After(5 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			// Process didn't exit, send SIGKILL
			// Process may already be dead; ignore SIGKILL error.
			_ = process.Signal(syscall.SIGKILL)
			// Wait a bit for SIGKILL to take effect
			time.Sleep(500 * time.Millisecond)
			// Clean up PID file
			_ = RemovePID()
			return nil
		case <-ticker.C:
			// Check if process still exists
			if err := process.Signal(syscall.Signal(0)); err != nil {
				// Process is gone
				_ = RemovePID()
				return nil
			}
		}
	}
}
