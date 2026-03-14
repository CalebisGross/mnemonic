package filesystem

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// knownNoisyApp maps a directory name (found under XDG/Library base dirs)
// to a human-readable description of why it's noisy.
type knownNoisyApp struct {
	Dir         string // directory name to match (e.g., "Code", "google-chrome")
	Description string // why it's noisy
}

// Registry of known noisy applications. These write high-frequency internal
// state to disk that is never useful as developer memories.
var knownNoisyApps = []knownNoisyApp{
	// Browsers
	{Dir: "google-chrome", Description: "Chrome browser storage"},
	{Dir: "chromium", Description: "Chromium browser storage"},
	{Dir: "BraveSoftware", Description: "Brave browser storage"},
	{Dir: "firefox", Description: "Firefox browser storage"},
	{Dir: "vivaldi", Description: "Vivaldi browser storage"},
	{Dir: "opera", Description: "Opera browser storage"},

	// Editors/IDEs
	{Dir: "Code", Description: "VS Code internal state"},
	{Dir: "Code - Insiders", Description: "VS Code Insiders internal state"},
	{Dir: "Cursor", Description: "Cursor editor internal state"},
	{Dir: "JetBrains", Description: "JetBrains IDE state"},

	// Communication
	{Dir: "Slack", Description: "Slack desktop state"},
	{Dir: "discord", Description: "Discord desktop state"},
	{Dir: "Signal", Description: "Signal messenger state"},
	{Dir: "teams", Description: "MS Teams state"},
	{Dir: "Microsoft Teams", Description: "MS Teams state"},
	{Dir: "Telegram Desktop", Description: "Telegram state"},
	{Dir: "zoom.us", Description: "Zoom state"},

	// Media/Desktop
	{Dir: "spotify", Description: "Spotify cache"},
	{Dir: "Spotify", Description: "Spotify cache"},
	{Dir: "vlc", Description: "VLC media player state"},

	// Desktop environments
	{Dir: "gnome-shell", Description: "GNOME shell temp files"},
	{Dir: "plasma", Description: "KDE Plasma state"},
	{Dir: "xfce4", Description: "XFCE desktop state"},
	{Dir: "cinnamon", Description: "Cinnamon desktop state"},

	// System services
	{Dir: "dconf", Description: "GNOME settings backend"},
	{Dir: "gconf", Description: "legacy GNOME settings"},
	{Dir: "pulse", Description: "PulseAudio state"},
	{Dir: "pipewire", Description: "PipeWire audio state"},

	// Package managers / runtimes
	{Dir: "yarn", Description: "Yarn package cache"},
	{Dir: "pnpm", Description: "pnpm package cache"},
	{Dir: "Docker Desktop", Description: "Docker Desktop state"},

	// Cloud sync / misc
	{Dir: "Dropbox", Description: "Dropbox sync state"},
	{Dir: "OneDrive", Description: "OneDrive sync state"},
	{Dir: "obsidian", Description: "Obsidian vault metadata"},
	{Dir: "1Password", Description: "1Password state"},
}

// windowsBaseDirs returns the base directories to scan on Windows.
func windowsBaseDirs() []string {
	var dirs []string
	if appData := os.Getenv("APPDATA"); appData != "" {
		dirs = append(dirs, appData)
	}
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		dirs = append(dirs, localAppData)
	}
	return dirs
}

// linuxBaseDirs returns the XDG base directories to scan on Linux.
func linuxBaseDirs(home string) []string {
	return []string{
		filepath.Join(home, ".config"),
		filepath.Join(home, ".local", "share"),
	}
}

// darwinBaseDirs returns the base directories to scan on macOS.
func darwinBaseDirs(home string) []string {
	return []string{
		filepath.Join(home, "Library", "Application Support"),
		filepath.Join(home, "Library", "Caches"),
	}
}

// DetectNoisyApps scans known base directories for installed applications
// that are known to produce high-frequency filesystem noise. Returns
// exclusion patterns for any that are found.
func DetectNoisyApps(log *slog.Logger) []string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Warn("auto-detect: could not determine home directory", "error", err)
		return nil
	}

	var baseDirs []string
	switch runtime.GOOS {
	case "linux":
		baseDirs = linuxBaseDirs(home)
	case "darwin":
		baseDirs = darwinBaseDirs(home)
	case "windows":
		baseDirs = windowsBaseDirs()
	default:
		log.Debug("auto-detect: unsupported platform, skipping", "os", runtime.GOOS)
		return nil
	}

	// Build a lookup set from existing config patterns so we don't duplicate
	var detected []string

	for _, baseDir := range baseDirs {
		for _, app := range knownNoisyApps {
			candidate := filepath.Join(baseDir, app.Dir)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				// Use the path relative to home for the exclusion pattern,
				// with trailing slash to match the substring convention
				relPattern := "." + candidate[len(home):]
				if relPattern[len(relPattern)-1] != filepath.Separator {
					relPattern += string(filepath.Separator)
				}
				detected = append(detected, relPattern)
				log.Info("auto-detected noisy app",
					"path", candidate,
					"pattern", relPattern,
					"description", app.Description,
				)
			}
		}
	}

	if len(detected) > 0 {
		log.Info("auto-detect complete", "exclusions_found", len(detected))
	} else {
		log.Debug("auto-detect: no additional noisy apps found")
	}

	return detected
}
