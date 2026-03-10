package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
