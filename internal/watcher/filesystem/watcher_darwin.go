//go:build darwin

package filesystem

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsevents"
	"github.com/google/uuid"

	"github.com/appsprout/mnemonic/internal/watcher"
)

// FilesystemWatcher implements the watcher.Watcher interface using macOS FSEvents.
// FSEvents can efficiently watch entire directory trees with a single stream,
// unlike kqueue-based fsnotify which requires one file descriptor per directory.
type FilesystemWatcher struct {
	cfg     Config
	log     *slog.Logger
	streams []*fsevents.EventStream
	events  chan watcher.Event
	done    chan struct{}
	mu      sync.RWMutex
	running bool

	// Debounce rapid changes to the same file
	debounce map[string]*time.Timer
	dbMutex  sync.Mutex
}

// NewFilesystemWatcher creates a new macOS FSEvents-based filesystem watcher.
func NewFilesystemWatcher(cfg Config, log *slog.Logger) (*FilesystemWatcher, error) {
	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	return &FilesystemWatcher{
		cfg:      cfg,
		log:      log,
		events:   make(chan watcher.Event, 100),
		done:     make(chan struct{}),
		debounce: make(map[string]*time.Timer),
	}, nil
}

func (fw *FilesystemWatcher) Name() string {
	return "filesystem"
}

func (fw *FilesystemWatcher) Start(ctx context.Context) error {
	fw.mu.Lock()
	if fw.running {
		fw.mu.Unlock()
		return fmt.Errorf("watcher is already running")
	}
	fw.running = true
	fw.mu.Unlock()

	// Create one FSEvents stream per configured watch directory.
	// Each stream recursively watches the entire tree — no per-directory FD needed.
	for _, dir := range fw.cfg.WatchDirs {
		es := &fsevents.EventStream{
			Paths:   []string{dir},
			Latency: 500 * time.Millisecond, // Coalesce rapid changes (acts as debounce)
			Flags:   fsevents.FileEvents | fsevents.WatchRoot,
		}

		es.Start()
		fw.streams = append(fw.streams, es)

		// Process events from this stream
		go fw.handleStream(ctx, es)

		fw.log.Info("fsevents stream started", "dir", dir)
	}

	fw.log.Info("filesystem watcher started (fsevents)", "dirs", fw.cfg.WatchDirs)
	return nil
}

func (fw *FilesystemWatcher) Stop() error {
	fw.mu.Lock()
	if !fw.running {
		fw.mu.Unlock()
		return fmt.Errorf("watcher is not running")
	}
	fw.running = false
	fw.mu.Unlock()

	close(fw.done)

	// Stop all FSEvents streams
	for _, es := range fw.streams {
		es.Stop()
	}
	fw.streams = nil

	// Clean up debounce timers
	fw.dbMutex.Lock()
	for _, timer := range fw.debounce {
		timer.Stop()
	}
	fw.dbMutex.Unlock()

	close(fw.events)
	fw.log.Info("filesystem watcher stopped")
	return nil
}

func (fw *FilesystemWatcher) Events() <-chan watcher.Event {
	return fw.events
}

// AddExclusion adds an exclusion pattern at runtime. Thread-safe.
func (fw *FilesystemWatcher) AddExclusion(pattern string) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	fw.cfg.ExcludePatterns = append(fw.cfg.ExcludePatterns, pattern)
}

func (fw *FilesystemWatcher) Health(ctx context.Context) error {
	fw.mu.RLock()
	defer fw.mu.RUnlock()
	if !fw.running {
		return fmt.Errorf("watcher is not running")
	}
	return nil
}

// handleStream processes events from a single FSEvents stream.
func (fw *FilesystemWatcher) handleStream(ctx context.Context, es *fsevents.EventStream) {
	for {
		select {
		case <-fw.done:
			return
		case <-ctx.Done():
			return
		case events, ok := <-es.Events:
			if !ok {
				return
			}
			for _, event := range events {
				fw.processEvent(event)
			}
		}
	}
}

// processEvent converts an FSEvents event into a watcher.Event and enqueues it.
func (fw *FilesystemWatcher) processEvent(event fsevents.Event) {
	path := "/" + event.Path // FSEvents strips leading slash

	// Skip excluded paths
	fw.mu.RLock()
	excluded := MatchesExcludePattern(path, fw.cfg.ExcludePatterns)
	sensitive := len(fw.cfg.SensitivePatterns) > 0 && IsSensitiveFile(path, fw.cfg.SensitivePatterns)
	fw.mu.RUnlock()
	if excluded || sensitive {
		return
	}

	// Determine event type from FSEvents flags
	eventType := fw.classifyEvent(event.Flags, event.Path)
	if eventType == "" {
		return // Not an event type we care about
	}

	// Debounce: coalesce rapid changes to the same path
	fw.dbMutex.Lock()
	if existingTimer, ok := fw.debounce[path]; ok {
		existingTimer.Stop()
	}

	// Capture values for the closure
	capturedType := eventType
	capturedPath := path

	fw.debounce[path] = time.AfterFunc(300*time.Millisecond, func() {
		fw.sendEvent(capturedPath, capturedType)
		fw.dbMutex.Lock()
		delete(fw.debounce, capturedPath)
		fw.dbMutex.Unlock()
	})
	fw.dbMutex.Unlock()
}

// classifyEvent maps FSEvents flags to our event type strings.
func (fw *FilesystemWatcher) classifyEvent(flags fsevents.EventFlags, path string) string {
	// FSEvents can report multiple flags at once.
	if flags&fsevents.ItemRemoved != 0 {
		return "file_deleted"
	}
	if flags&fsevents.ItemCreated != 0 {
		// If both Created and Renamed are set, macOS did an atomic save
		// (write temp → rename over original). Treat as modified.
		if flags&fsevents.ItemRenamed != 0 {
			return "file_modified"
		}
		return "file_created"
	}
	if flags&fsevents.ItemRenamed != 0 {
		// A standalone rename where the file still exists at this path
		// is an atomic save — treat as modified.
		if _, err := os.Stat("/" + path); err == nil {
			return "file_modified"
		}
		return "file_renamed"
	}
	if flags&fsevents.ItemModified != 0 {
		return "file_modified"
	}
	if flags&fsevents.ItemInodeMetaMod != 0 {
		// Metadata-only changes (permissions, etc.) — skip
		return ""
	}
	return ""
}

// sendEvent builds and emits the final watcher.Event.
func (fw *FilesystemWatcher) sendEvent(path string, eventType string) {
	var content string

	// Read file content for create/modify/rename events on non-binary regular files.
	// Rename is included because macOS editors use atomic saves (write temp → rename over original).
	if (eventType == "file_created" || eventType == "file_modified" || eventType == "file_renamed") && !IsBinaryFile(path) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			content = ReadFileContent(path, fw.cfg.MaxContentBytes, fw.log)
			// If content is empty, the file may not be fully flushed yet — retry once after a short delay
			if content == "" {
				time.Sleep(100 * time.Millisecond)
				content = ReadFileContent(path, fw.cfg.MaxContentBytes, fw.log)
			}
			// Safety net: detect binary content even when the extension check missed it
			if content != "" && IsBinaryContent(content) {
				fw.log.Debug("dropping binary content detected at runtime", "path", path)
				content = ""
			}
		}
	}

	wevent := watcher.Event{
		ID:        uuid.New().String(),
		Source:    "filesystem",
		Type:      eventType,
		Path:      path,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"op":      eventType,
			"backend": "fsevents",
		},
	}

	select {
	case fw.events <- wevent:
		fw.log.Debug("sent filesystem event", "type", eventType, "path", path)
	case <-fw.done:
		return
	}
}
