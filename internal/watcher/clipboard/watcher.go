package clipboard

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/watcher"
	"github.com/google/uuid"
)

// Config holds configuration for the clipboard watcher.
type Config struct {
	PollIntervalSec int // polling interval in seconds
	MaxContentBytes int // maximum clipboard content size to capture
}

// ClipboardWatcher implements the watcher.Watcher interface for clipboard changes.
type ClipboardWatcher struct {
	cfg                Config
	log                *slog.Logger
	events             chan watcher.Event
	done               chan struct{}
	mu                 sync.RWMutex
	running            bool
	lastContentHash    string
	lastNContentHashes []string // track last N contents to avoid duplicates
	maxHistorySize     int      // number of previous clips to track
	enabled            bool     // whether clipboard reading is supported on this platform
}

// NewClipboardWatcher creates a new clipboard watcher.
func NewClipboardWatcher(cfg Config, log *slog.Logger) (*ClipboardWatcher, error) {
	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	if cfg.PollIntervalSec <= 0 {
		cfg.PollIntervalSec = 1
	}

	if cfg.MaxContentBytes <= 0 {
		cfg.MaxContentBytes = 1024 * 1024 // 1MB default
	}

	cw := &ClipboardWatcher{
		cfg:                cfg,
		log:                log,
		events:             make(chan watcher.Event, 100),
		done:               make(chan struct{}),
		maxHistorySize:     3,
		lastNContentHashes: make([]string, 0, 3),
		enabled:            true,
	}

	return cw, nil
}

// Name returns the watcher's name.
func (cw *ClipboardWatcher) Name() string {
	return "clipboard"
}

// Start begins watching the clipboard.
func (cw *ClipboardWatcher) Start(ctx context.Context) error {
	cw.mu.Lock()
	if cw.running {
		cw.mu.Unlock()
		return fmt.Errorf("watcher is already running")
	}

	// Check if clipboard reading is supported
	if !cw.isClipboardSupported() {
		cw.enabled = false
		cw.mu.Unlock()
		cw.log.Warn("clipboard reading not supported on this platform")
		return fmt.Errorf("clipboard reading not supported on %s", runtime.GOOS)
	}

	cw.running = true
	cw.mu.Unlock()

	// Get baseline clipboard content
	if content, err := cw.readClipboard(); err == nil && content != "" {
		cw.lastContentHash = cw.hashContent(content)
		cw.trackContentHash(cw.lastContentHash)
	}

	// Start polling goroutine
	go cw.pollClipboard(ctx)

	cw.log.Info("clipboard watcher started")
	return nil
}

// Stop gracefully stops the watcher.
func (cw *ClipboardWatcher) Stop() error {
	cw.mu.Lock()
	if !cw.running {
		cw.mu.Unlock()
		return fmt.Errorf("watcher is not running")
	}
	cw.running = false
	cw.mu.Unlock()

	close(cw.done)
	close(cw.events)

	cw.log.Info("clipboard watcher stopped")
	return nil
}

// Events returns a read-only channel of events.
func (cw *ClipboardWatcher) Events() <-chan watcher.Event {
	return cw.events
}

// Health checks if the watcher is functioning.
func (cw *ClipboardWatcher) Health(ctx context.Context) error {
	cw.mu.RLock()
	defer cw.mu.RUnlock()

	if !cw.running {
		return fmt.Errorf("watcher is not running")
	}

	if !cw.enabled {
		return fmt.Errorf("clipboard reading not available on this platform")
	}

	return nil
}

// isClipboardSupported checks if the platform supports clipboard reading.
func (cw *ClipboardWatcher) isClipboardSupported() bool {
	switch runtime.GOOS {
	case "darwin": // macOS
		return cw.checkCommandExists("pbpaste")
	case "linux":
		return cw.checkCommandExists("xclip") || cw.checkCommandExists("xsel")
	case "windows":
		return cw.checkCommandExists("powershell") || cw.checkCommandExists("Get-Clipboard")
	default:
		return false
	}
}

// checkCommandExists checks if a command is available in PATH.
func (cw *ClipboardWatcher) checkCommandExists(cmd string) bool {
	_, err := exec.LookPath(cmd)
	return err == nil
}

// readClipboard reads the current clipboard content.
func (cw *ClipboardWatcher) readClipboard() (string, error) {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbpaste")
	case "linux":
		// Try xclip first, then xsel
		if cw.checkCommandExists("xclip") {
			cmd = exec.Command("xclip", "-selection", "clipboard", "-o")
		} else if cw.checkCommandExists("xsel") {
			cmd = exec.Command("xsel", "--clipboard", "--output")
		} else {
			return "", fmt.Errorf("no clipboard tool available (xclip or xsel required)")
		}
	case "windows":
		// Use PowerShell Get-Clipboard
		cmd = exec.Command("powershell", "-Command", "Get-Clipboard")
	default:
		return "", fmt.Errorf("clipboard reading not supported on %s", runtime.GOOS)
	}

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// pollClipboard periodically checks for clipboard changes.
func (cw *ClipboardWatcher) pollClipboard(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(cw.cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cw.done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if cw.enabled {
				cw.checkForClipboardChange()
			}
		}
	}
}

// checkForClipboardChange reads clipboard and sends event if content changed.
func (cw *ClipboardWatcher) checkForClipboardChange() {
	content, err := cw.readClipboard()
	if err != nil {
		cw.log.Debug("failed to read clipboard", "err", err)
		return
	}

	// Skip empty content
	if content == "" {
		return
	}

	// Skip if too large (likely binary data)
	if len(content) > cw.cfg.MaxContentBytes {
		cw.log.Debug("clipboard content exceeds size limit", "size", len(content), "max", cw.cfg.MaxContentBytes)
		return
	}

	// Hash the new content
	contentHash := cw.hashContent(content)

	cw.mu.Lock()
	defer cw.mu.Unlock()

	// Skip if hash matches last content
	if contentHash == cw.lastContentHash {
		return
	}

	// Skip if we've seen this content in the last N clipboard entries
	if cw.isContentInHistory(contentHash) {
		return
	}

	// Update tracking
	cw.lastContentHash = contentHash
	cw.trackContentHash(contentHash)

	// Send event (unlock to avoid blocking on channel send)
	cw.mu.Unlock()
	cw.sendEvent(content)
	cw.mu.Lock()
}

// hashContent computes a hash of the content.
func (cw *ClipboardWatcher) hashContent(content string) string {
	h := md5.New()
	_, _ = io.WriteString(h, content)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// trackContentHash adds a hash to the history of clipboard contents.
func (cw *ClipboardWatcher) trackContentHash(hash string) {
	if len(cw.lastNContentHashes) >= cw.maxHistorySize {
		// Remove oldest entry
		cw.lastNContentHashes = cw.lastNContentHashes[1:]
	}
	cw.lastNContentHashes = append(cw.lastNContentHashes, hash)
}

// isContentInHistory checks if a content hash is in the recent history.
func (cw *ClipboardWatcher) isContentInHistory(hash string) bool {
	for _, h := range cw.lastNContentHashes {
		if h == hash {
			return true
		}
	}
	return false
}

// sendEvent sends a watcher.Event on the events channel.
func (cw *ClipboardWatcher) sendEvent(content string) {
	event := watcher.Event{
		ID:        uuid.New().String(),
		Source:    "clipboard",
		Type:      "clipboard_changed",
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"platform": runtime.GOOS,
			"size":     len(content),
		},
	}

	select {
	case cw.events <- event:
		cw.log.Debug("sent clipboard event", "type", event.Type, "size", len(content))
	case <-cw.done:
		return
	}
}
