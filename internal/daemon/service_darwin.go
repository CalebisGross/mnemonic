//go:build darwin

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

const serviceLabel = "com.appsprout.mnemonic"

const launchAgentPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.appsprout.mnemonic</string>
	<key>ProgramArguments</key>
	<array>
		<string>{{.ExecPath}}</string>
		<string>--config</string>
		<string>{{.ConfigPath}}</string>
		<string>serve</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>StandardOutPath</key>
	<string>{{.LogPath}}</string>
	<key>StandardErrorPath</key>
	<string>{{.LogPath}}</string>
	<key>WorkingDirectory</key>
	<string>{{.HomeDir}}</string>
	<key>EnvironmentVariables</key>
	<dict>
		<key>PATH</key>
		<string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
	</dict>
</dict>
</plist>
`

type launchdManager struct{}

// NewServiceManager returns the macOS launchd service manager.
func NewServiceManager() ServiceManager {
	return &launchdManager{}
}

func (m *launchdManager) IsInstalled() bool {
	cmd := exec.Command("launchctl", "list", serviceLabel)
	return cmd.Run() == nil
}

func (m *launchdManager) IsRunning() (bool, int) {
	out, err := exec.Command("launchctl", "list", serviceLabel).Output()
	if err != nil {
		return false, 0
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "\"PID\"") {
			parts := strings.Split(line, "=")
			if len(parts) == 2 {
				numStr := strings.TrimSpace(parts[1])
				numStr = strings.TrimSuffix(numStr, ";")
				numStr = strings.TrimSpace(numStr)
				if pid, err := strconv.Atoi(numStr); err == nil && pid > 0 {
					return true, pid
				}
			}
		}
	}
	return false, 0
}

func (m *launchdManager) Install(execPath, configPath string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	// Resolve symlinks on the exec path
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	tmpl, err := template.New("plist").Parse(launchAgentPlist)
	if err != nil {
		return fmt.Errorf("parsing plist template: %w", err)
	}

	data := struct {
		ExecPath   string
		ConfigPath string
		LogPath    string
		HomeDir    string
	}{
		ExecPath:   execPath,
		ConfigPath: configPath,
		LogPath:    LogPath(),
		HomeDir:    homeDir,
	}

	var plistContent strings.Builder
	if err := tmpl.Execute(&plistContent, data); err != nil {
		return fmt.Errorf("generating plist: %w", err)
	}

	launchAgentsDir := filepath.Join(homeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("creating LaunchAgents directory: %w", err)
	}
	logDir := filepath.Dir(LogPath())
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("creating log directory: %w", err)
	}

	plistPath := filepath.Join(launchAgentsDir, serviceLabel+".plist")
	if err := os.WriteFile(plistPath, []byte(plistContent.String()), 0644); err != nil {
		return fmt.Errorf("writing plist: %w", err)
	}

	return nil
}

func (m *launchdManager) Uninstall() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	plistPath := filepath.Join(homeDir, "Library", "LaunchAgents", serviceLabel+".plist")

	// Try to unload first (may fail if not loaded, that's fine)
	if _, err := os.Stat(plistPath); err == nil {
		unload := exec.Command("launchctl", "unload", plistPath)
		_ = unload.Run()
	}

	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing plist: %w", err)
	}

	return nil
}

func (m *launchdManager) Start() error {
	return exec.Command("launchctl", "start", serviceLabel).Run()
}

func (m *launchdManager) Stop() error {
	return exec.Command("launchctl", "stop", serviceLabel).Run()
}

func (m *launchdManager) Restart() error {
	// launchctl has no native restart; spawn a background shell to stop+start.
	// Start() (not Run) so the command outlives the current process.
	return exec.Command("sh", "-c",
		"launchctl stop "+serviceLabel+" && sleep 1 && launchctl start "+serviceLabel).Start()
}

func (m *launchdManager) ServiceName() string {
	return "launchd"
}
