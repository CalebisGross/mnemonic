//go:build !darwin

package filesystem

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/watcher"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

// FilesystemWatcher implements the watcher.Watcher interface using fsnotify (kqueue/inotify).
// Used on non-macOS platforms. On macOS, the FSEvents-based watcher is used instead.
type FilesystemWatcher struct {
	cfg      Config
	log      *slog.Logger
	fsw      *fsnotify.Watcher
	events   chan watcher.Event
	done     chan struct{}
	mu       sync.RWMutex
	running  bool
	debounce map[string]*time.Timer
	dbMutex  sync.Mutex
}

// NewFilesystemWatcher creates a new filesystem watcher using fsnotify.
func NewFilesystemWatcher(cfg Config, log *slog.Logger) (*FilesystemWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	if log == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	return &FilesystemWatcher{
		cfg:      cfg,
		log:      log,
		fsw:      fsw,
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

	for _, dir := range fw.cfg.WatchDirs {
		if err := fw.addDirRecursive(dir); err != nil {
			fw.log.Warn("failed to add directory", "dir", dir, "err", err)
		}
	}

	go fw.handleEvents(ctx)

	fw.log.Info("filesystem watcher started (fsnotify)", "dirs", fw.cfg.WatchDirs)
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

	fw.dbMutex.Lock()
	for _, timer := range fw.debounce {
		timer.Stop()
	}
	fw.dbMutex.Unlock()

	if err := fw.fsw.Close(); err != nil {
		fw.log.Error("closing fsnotify watcher", "err", err)
		return err
	}

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

func (fw *FilesystemWatcher) addDirRecursive(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		fw.mu.RLock()
		excluded := MatchesExcludePattern(path, fw.cfg.ExcludePatterns)
		fw.mu.RUnlock()
		if excluded {
			return filepath.SkipDir
		}
		if err := fw.fsw.Add(path); err != nil {
			fw.log.Debug("failed to add directory to watcher", "path", path, "err", err)
			return nil
		}
		fw.log.Debug("added directory to watcher", "path", path)
		return nil
	})
}

func (fw *FilesystemWatcher) handleEvents(ctx context.Context) {
	for {
		select {
		case <-fw.done:
			return
		case <-ctx.Done():
			return
		case event, ok := <-fw.fsw.Events:
			if !ok {
				return
			}
			fw.mu.RLock()
			excluded := MatchesExcludePattern(event.Name, fw.cfg.ExcludePatterns)
			fw.mu.RUnlock()
			if excluded {
				continue
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if err := fw.addDirRecursive(event.Name); err != nil {
						fw.log.Debug("failed to add new directory to watcher", "path", event.Name, "err", err)
					}
				}
			}
			fw.enqueueEvent(event)
		case err, ok := <-fw.fsw.Errors:
			if !ok {
				return
			}
			fw.log.Error("fsnotify error", "err", err)
		}
	}
}

func (fw *FilesystemWatcher) enqueueEvent(event fsnotify.Event) {
	path := event.Name

	fw.dbMutex.Lock()
	defer fw.dbMutex.Unlock()

	if existingTimer, ok := fw.debounce[path]; ok {
		existingTimer.Stop()
	}

	fw.debounce[path] = time.AfterFunc(500*time.Millisecond, func() {
		fw.sendEvent(event)
		fw.dbMutex.Lock()
		delete(fw.debounce, path)
		fw.dbMutex.Unlock()
	})
}

func (fw *FilesystemWatcher) sendEvent(event fsnotify.Event) {
	var eventType string
	if event.Has(fsnotify.Create) {
		eventType = "file_created"
	} else if event.Has(fsnotify.Write) {
		eventType = "file_modified"
	} else if event.Has(fsnotify.Remove) {
		eventType = "file_deleted"
	} else if event.Has(fsnotify.Rename) {
		eventType = "file_renamed"
	} else {
		return
	}

	var content string
	if (event.Has(fsnotify.Create) || event.Has(fsnotify.Write)) && !IsBinaryFile(event.Name) {
		if info, err := os.Stat(event.Name); err == nil && info.Mode().IsRegular() {
			content = ReadFileContent(event.Name, fw.cfg.MaxContentBytes, fw.log)
			// Safety net: detect binary content even when the extension check missed it
			if content != "" && IsBinaryContent(content) {
				fw.log.Debug("dropping binary content detected at runtime", "path", event.Name)
				content = ""
			}
		}
	}

	wevent := watcher.Event{
		ID:        uuid.New().String(),
		Source:    "filesystem",
		Type:      eventType,
		Path:      event.Name,
		Content:   content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"op": event.Op.String(),
		},
	}

	select {
	case fw.events <- wevent:
		fw.log.Debug("sent filesystem event", "type", eventType, "path", event.Name)
	case <-fw.done:
		return
	}
}
