package terminal

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/watcher"
	"github.com/google/uuid"
)

// Config holds configuration for the terminal watcher.
type Config struct {
	Shell           string   // "auto", "bash", "zsh", "fish"
	PollIntervalSec int      // polling interval in seconds
	ExcludePatterns []string // regex patterns to skip
}

// TerminalWatcher implements the watcher.Watcher interface for shell history.
type TerminalWatcher struct {
	cfg              Config
	log              *slog.Logger
	events           chan watcher.Event
	done             chan struct{}
	mu               sync.RWMutex
	running          bool
	historyFilePath  string
	lastOffset       int64
	excludeRegexps   []*regexp.Regexp
	platformDetector func() string // for testing
}

// NewTerminalWatcher creates a new terminal watcher.
func NewTerminalWatcher(cfg Config, log *slog.Logger) (*TerminalWatcher, error) {
	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	if cfg.PollIntervalSec <= 0 {
		cfg.PollIntervalSec = 1
	}

	// Compile exclude patterns
	var excludeRegexps []*regexp.Regexp
	for _, pattern := range cfg.ExcludePatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid exclude pattern %q: %w", pattern, err)
		}
		excludeRegexps = append(excludeRegexps, re)
	}

	tw := &TerminalWatcher{
		cfg:            cfg,
		log:            log,
		events:         make(chan watcher.Event, 100),
		done:           make(chan struct{}),
		excludeRegexps: excludeRegexps,
		platformDetector: func() string {
			if shell := os.Getenv("SHELL"); shell != "" {
				return shell
			}
			if runtime.GOOS == "windows" {
				// Git Bash on Windows sets MSYSTEM
				if os.Getenv("MSYSTEM") != "" {
					return "bash"
				}
				return "powershell"
			}
			return ""
		},
	}

	return tw, nil
}

// Name returns the watcher's name.
func (tw *TerminalWatcher) Name() string {
	return "terminal"
}

// Start begins watching shell history.
func (tw *TerminalWatcher) Start(ctx context.Context) error {
	tw.mu.Lock()
	if tw.running {
		tw.mu.Unlock()
		return fmt.Errorf("watcher is already running")
	}

	// Detect shell if needed
	shell := tw.cfg.Shell
	if shell == "auto" || shell == "" {
		shellEnv := tw.platformDetector()
		if shellEnv != "" {
			// Extract just the shell name from the full path (e.g., /bin/bash -> bash)
			shell = filepath.Base(shellEnv)
		} else {
			shell = "bash" // default fallback
		}
	}

	// Determine history file path
	historyPath, err := tw.getHistoryFilePath(shell)
	if err != nil {
		tw.mu.Unlock()
		return fmt.Errorf("determining history file path: %w", err)
	}

	tw.historyFilePath = historyPath
	tw.running = true
	tw.mu.Unlock()

	// Get initial offset
	if err := tw.updateInitialOffset(); err != nil {
		tw.log.Warn("failed to get initial offset", "path", tw.historyFilePath, "err", err)
	}

	// Start polling goroutine
	go tw.pollHistory(ctx)

	tw.log.Info("terminal watcher started", "shell", shell, "history_file", tw.historyFilePath)
	return nil
}

// Stop gracefully stops the watcher.
func (tw *TerminalWatcher) Stop() error {
	tw.mu.Lock()
	if !tw.running {
		tw.mu.Unlock()
		return fmt.Errorf("watcher is not running")
	}
	tw.running = false
	tw.mu.Unlock()

	close(tw.done)
	close(tw.events)

	tw.log.Info("terminal watcher stopped")
	return nil
}

// Events returns a read-only channel of events.
func (tw *TerminalWatcher) Events() <-chan watcher.Event {
	return tw.events
}

// Health checks if the watcher is functioning.
func (tw *TerminalWatcher) Health(ctx context.Context) error {
	tw.mu.RLock()
	defer tw.mu.RUnlock()

	if !tw.running {
		return fmt.Errorf("watcher is not running")
	}

	return nil
}

// getHistoryFilePath returns the path to the shell history file.
func (tw *TerminalWatcher) getHistoryFilePath(shell string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	switch strings.ToLower(shell) {
	case "bash":
		return filepath.Join(homeDir, ".bash_history"), nil
	case "zsh":
		return filepath.Join(homeDir, ".zsh_history"), nil
	case "fish":
		return filepath.Join(homeDir, ".local", "share", "fish", "fish_history"), nil
	case "powershell", "pwsh":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			return "", fmt.Errorf("APPDATA environment variable not set")
		}
		return filepath.Join(appData, "Microsoft", "Windows", "PowerShell", "PSReadLine", "ConsoleHost_history.txt"), nil
	default:
		return filepath.Join(homeDir, ".bash_history"), nil // default to bash
	}
}

// updateInitialOffset reads the current file size to set baseline.
func (tw *TerminalWatcher) updateInitialOffset() error {
	info, err := os.Stat(tw.historyFilePath)
	if err != nil {
		// File doesn't exist yet, that's ok
		tw.lastOffset = 0
		return nil
	}

	tw.lastOffset = info.Size()
	return nil
}

// pollHistory periodically checks for new history entries.
func (tw *TerminalWatcher) pollHistory(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(tw.cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-tw.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			tw.checkForNewEntries()
		}
	}
}

// checkForNewEntries reads the history file and sends events for new entries.
func (tw *TerminalWatcher) checkForNewEntries() {
	file, err := os.Open(tw.historyFilePath)
	if err != nil {
		tw.log.Debug("failed to open history file", "path", tw.historyFilePath, "err", err)
		return
	}
	defer func() { _ = file.Close() }()

	// Get current file size
	info, err := file.Stat()
	if err != nil {
		tw.log.Debug("failed to stat history file", "path", tw.historyFilePath, "err", err)
		return
	}

	currentSize := info.Size()

	// Only read if file has grown
	if currentSize <= tw.lastOffset {
		return
	}

	// Seek to last offset
	if _, err := file.Seek(tw.lastOffset, 0); err != nil {
		tw.log.Debug("failed to seek in history file", "path", tw.historyFilePath, "err", err)
		return
	}

	// Read new content
	content, err := io.ReadAll(file)
	if err != nil {
		tw.log.Debug("failed to read history file", "path", tw.historyFilePath, "err", err)
		return
	}

	// Update offset for next time
	tw.lastOffset = currentSize

	// Process new lines
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse the command from the line
		command := tw.parseHistoryLine(line)
		if command == "" {
			continue
		}

		// Check if matches exclude pattern
		if tw.matchesExcludePattern(command) {
			continue
		}

		// Send event
		tw.sendEvent(command)
	}
}

// parseHistoryLine extracts the command from a history line.
// For zsh, lines are prefixed with `: timestamp:0;` format.
func (tw *TerminalWatcher) parseHistoryLine(line string) string {
	// Handle zsh history format: `: timestamp:0;command`
	if strings.HasPrefix(line, ":") {
		parts := strings.SplitN(line, ";", 2)
		if len(parts) == 2 {
			return strings.TrimSpace(parts[1])
		}
	}

	return line
}

// matchesExcludePattern checks if a command matches any exclude pattern.
func (tw *TerminalWatcher) matchesExcludePattern(command string) bool {
	for _, re := range tw.excludeRegexps {
		if re.MatchString(command) {
			return true
		}
	}
	return false
}

// sendEvent sends a watcher.Event on the events channel.
func (tw *TerminalWatcher) sendEvent(command string) {
	event := watcher.Event{
		ID:        uuid.New().String(),
		Source:    "terminal",
		Type:      "command_executed",
		Content:   command,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"shell": tw.cfg.Shell,
		},
	}

	select {
	case tw.events <- event:
		tw.log.Debug("sent terminal event", "type", event.Type, "content", command)
	case <-tw.done:
		return
	}
}
