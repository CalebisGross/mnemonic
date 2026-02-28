//go:build linux

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

const serviceName = "mnemonic"

const systemdUnitTemplate = `[Unit]
Description=Mnemonic - Semantic Memory System
After=network.target

[Service]
Type=simple
ExecStart={{.ExecPath}} --config {{.ConfigPath}} serve
Restart=on-failure
RestartSec=5
StandardOutput=append:{{.LogPath}}
StandardError=append:{{.LogPath}}
Environment=PATH=/usr/local/bin:/usr/bin:/bin

[Install]
WantedBy=default.target
`

type systemdManager struct{}

// NewServiceManager returns the Linux systemd service manager.
func NewServiceManager() ServiceManager {
	return &systemdManager{}
}

func (m *systemdManager) unitFilePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(homeDir, ".config", "systemd", "user", serviceName+".service"), nil
}

func (m *systemdManager) IsInstalled() bool {
	unitPath, err := m.unitFilePath()
	if err != nil {
		return false
	}
	_, err = os.Stat(unitPath)
	return err == nil
}

func (m *systemdManager) IsRunning() (bool, int) {
	// Check if the service is active
	cmd := exec.Command("systemctl", "--user", "is-active", serviceName)
	out, err := cmd.Output()
	if err != nil {
		return false, 0
	}
	if strings.TrimSpace(string(out)) != "active" {
		return false, 0
	}

	// Get the main PID
	pidCmd := exec.Command("systemctl", "--user", "show", "-p", "MainPID", "--value", serviceName)
	pidOut, err := pidCmd.Output()
	if err != nil {
		return true, 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidOut)))
	if err != nil || pid == 0 {
		return true, 0
	}
	return true, pid
}

func (m *systemdManager) Install(execPath, configPath string) error {
	unitPath, err := m.unitFilePath()
	if err != nil {
		return fmt.Errorf("resolving unit file path: %w", err)
	}

	// Resolve symlinks on the exec path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	tmpl, err := template.New("unit").Parse(systemdUnitTemplate)
	if err != nil {
		return fmt.Errorf("parsing unit template: %w", err)
	}

	data := struct {
		ExecPath   string
		ConfigPath string
		LogPath    string
	}{
		ExecPath:   execPath,
		ConfigPath: configPath,
		LogPath:    LogPath(),
	}

	var unitContent strings.Builder
	if err := tmpl.Execute(&unitContent, data); err != nil {
		return fmt.Errorf("generating unit file: %w", err)
	}

	// Ensure directories exist
	unitDir := filepath.Dir(unitPath)
	if err := os.MkdirAll(unitDir, 0755); err != nil {
		return fmt.Errorf("creating systemd user directory: %w", err)
	}
	logDir := filepath.Dir(LogPath())
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	if err := os.WriteFile(unitPath, []byte(unitContent.String()), 0644); err != nil {
		return fmt.Errorf("writing unit file: %w", err)
	}

	// Reload systemd and enable the service
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}
	if err := exec.Command("systemctl", "--user", "enable", serviceName).Run(); err != nil {
		return fmt.Errorf("enabling service: %w", err)
	}

	return nil
}

func (m *systemdManager) Uninstall() error {
	// Stop and disable (ignore errors — service may not be running)
	_ = exec.Command("systemctl", "--user", "stop", serviceName).Run()
	_ = exec.Command("systemctl", "--user", "disable", serviceName).Run()

	unitPath, err := m.unitFilePath()
	if err != nil {
		return fmt.Errorf("resolving unit file path: %w", err)
	}

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing unit file: %w", err)
	}

	// Reload so systemd forgets the unit
	if err := exec.Command("systemctl", "--user", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("reloading systemd: %w", err)
	}

	return nil
}

func (m *systemdManager) Start() error {
	return exec.Command("systemctl", "--user", "start", serviceName).Run()
}

func (m *systemdManager) Stop() error {
	return exec.Command("systemctl", "--user", "stop", serviceName).Run()
}

func (m *systemdManager) ServiceName() string {
	return "systemd"
}
