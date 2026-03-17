//go:build !darwin

package filesystem

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/watcher"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
)

// FilesystemWatcher implements the watcher.Watcher interface using fsnotify (inotify).
// Uses attention-based watching: shallow inotify for real-time perception of active areas,
// background polling for deeper/cold directories. A hard watch budget prevents inotify
// exhaustion that would starve VS Code and other apps.
type FilesystemWatcher struct {
	cfg    Config
	log    *slog.Logger
	fsw    *fsnotify.Watcher
	events chan watcher.Event
	done   chan struct{}
	mu     sync.RWMutex

	running  bool
	debounce map[string]*time.Timer
	dbMutex  sync.Mutex

	// Watch budget tracking
	watchCount int
	maxWatches int

	// Hot/cold tier tracking
	hotDirs      map[string]time.Time // dir → last activity time (inotify-watched)
	coldDirs     []string             // dirs being polled (deeper than shallow depth)
	coldMtimes   map[string]time.Time // dir → last known mtime (for change detection)
	coldActivity map[string]int       // dir → change count in current poll window
	tierMu       sync.Mutex           // protects hot/cold state
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

	maxWatches := cfg.MaxWatches
	if maxWatches == 0 {
		maxWatches = 20000
	}

	return &FilesystemWatcher{
		cfg:          cfg,
		log:          log,
		fsw:          fsw,
		events:       make(chan watcher.Event, 100),
		done:         make(chan struct{}),
		debounce:     make(map[string]*time.Timer),
		maxWatches:   maxWatches,
		hotDirs:      make(map[string]time.Time),
		coldMtimes:   make(map[string]time.Time),
		coldActivity: make(map[string]int),
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

	shallowDepth := fw.cfg.ShallowDepth
	if shallowDepth == 0 {
		shallowDepth = 3
	}

	// Phase 1: Add shallow inotify watches (depth-limited)
	for _, dir := range fw.cfg.WatchDirs {
		fw.addDirShallow(dir, shallowDepth)
	}

	fw.log.Info("filesystem watcher: shallow watches added",
		"watch_count", fw.watchCount,
		"budget", fw.maxWatches,
		"shallow_depth", shallowDepth,
	)

	// Phase 2: Discover cold directories (deeper than shallow depth)
	for _, dir := range fw.cfg.WatchDirs {
		fw.discoverColdDirs(dir, shallowDepth)
	}

	fw.log.Info("filesystem watcher: cold directories discovered",
		"cold_count", len(fw.coldDirs),
	)

	// Start event handler
	go fw.handleEvents(ctx)

	// Start background polling for cold directories
	pollInterval := time.Duration(fw.cfg.PollIntervalSec) * time.Second
	if pollInterval == 0 {
		pollInterval = 45 * time.Second
	}
	go fw.pollColdDirs(ctx, pollInterval)

	// Start demotion checker
	demotionTimeout := time.Duration(fw.cfg.DemotionTimeoutMin) * time.Minute
	if demotionTimeout == 0 {
		demotionTimeout = 30 * time.Minute
	}
	go fw.demotionLoop(ctx, demotionTimeout)

	fw.log.Info("filesystem watcher started (attention-based)",
		"dirs", fw.cfg.WatchDirs,
		"hot_watches", fw.watchCount,
		"cold_dirs", len(fw.coldDirs),
	)
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

// addDirShallow adds inotify watches up to maxDepth levels deep.
func (fw *FilesystemWatcher) addDirShallow(root string, maxDepth int) {
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible dirs
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

		// Check depth
		pathDepth := strings.Count(filepath.Clean(path), string(filepath.Separator))
		if pathDepth-rootDepth >= maxDepth {
			return filepath.SkipDir
		}

		// Check budget
		if fw.watchCount >= fw.maxWatches {
			fw.log.Warn("inotify watch budget exhausted",
				"budget", fw.maxWatches,
				"path", path,
			)
			return filepath.SkipDir
		}

		if err := fw.fsw.Add(path); err != nil {
			fw.log.Debug("failed to add inotify watch", "path", path, "err", err)
			return nil
		}

		fw.watchCount++
		fw.tierMu.Lock()
		fw.hotDirs[path] = time.Now()
		fw.tierMu.Unlock()

		return nil
	})
}

// discoverColdDirs finds directories deeper than shallowDepth for background polling.
func (fw *FilesystemWatcher) discoverColdDirs(root string, shallowDepth int) {
	rootDepth := strings.Count(filepath.Clean(root), string(filepath.Separator))

	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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

		pathDepth := strings.Count(filepath.Clean(path), string(filepath.Separator))
		depth := pathDepth - rootDepth

		// Skip directories already watched by inotify (shallow tier)
		if depth < shallowDepth {
			return nil
		}

		// Only add directories at exactly shallowDepth as cold polling roots.
		// We don't need to recurse infinitely — poll these and they'll tell us
		// about activity in their subtrees via mtime changes.
		if depth == shallowDepth {
			fw.tierMu.Lock()
			fw.coldDirs = append(fw.coldDirs, path)
			// Record initial mtime
			if info, err := os.Stat(path); err == nil {
				fw.coldMtimes[path] = info.ModTime()
			}
			fw.tierMu.Unlock()
			return filepath.SkipDir // don't recurse deeper
		}

		return nil
	})
}

// pollColdDirs periodically checks cold directories for mtime changes.
func (fw *FilesystemWatcher) pollColdDirs(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	promotionThreshold := fw.cfg.PromotionThreshold
	if promotionThreshold == 0 {
		promotionThreshold = 3
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.done:
			return
		case <-ticker.C:
			fw.tierMu.Lock()
			var toPromote []string

			for _, dir := range fw.coldDirs {
				info, err := os.Stat(dir)
				if err != nil {
					continue
				}

				lastMtime, known := fw.coldMtimes[dir]
				currentMtime := info.ModTime()

				if known && currentMtime.After(lastMtime) {
					// Activity detected
					fw.coldActivity[dir]++
					fw.coldMtimes[dir] = currentMtime

					// Emit a polling-detected event for the changed directory
					fw.tierMu.Unlock()
					fw.emitPollEvent(dir)
					fw.tierMu.Lock()

					if fw.coldActivity[dir] >= promotionThreshold {
						toPromote = append(toPromote, dir)
					}
				} else if !known {
					fw.coldMtimes[dir] = currentMtime
				}
			}
			fw.tierMu.Unlock()

			// Promote active cold dirs to hot (inotify)
			for _, dir := range toPromote {
				fw.promoteToHot(dir)
			}
		}
	}
}

// promoteToHot moves a directory from cold polling to hot inotify watching.
func (fw *FilesystemWatcher) promoteToHot(dir string) {
	// Check budget
	if fw.watchCount >= fw.maxWatches {
		fw.log.Debug("cannot promote to hot: budget exhausted", "dir", dir)
		return
	}

	if err := fw.fsw.Add(dir); err != nil {
		fw.log.Debug("failed to promote directory to hot", "dir", dir, "err", err)
		return
	}

	fw.watchCount++

	fw.tierMu.Lock()
	defer fw.tierMu.Unlock()

	fw.hotDirs[dir] = time.Now()
	delete(fw.coldActivity, dir)

	// Remove from cold list
	for i, d := range fw.coldDirs {
		if d == dir {
			fw.coldDirs = append(fw.coldDirs[:i], fw.coldDirs[i+1:]...)
			break
		}
	}

	fw.log.Info("promoted directory to hot (inotify)",
		"dir", dir,
		"watch_count", fw.watchCount,
	)
}

// demotionLoop periodically checks for inactive hot directories and demotes them.
func (fw *FilesystemWatcher) demotionLoop(ctx context.Context, timeout time.Duration) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-fw.done:
			return
		case <-ticker.C:
			fw.demoteColdDirs(timeout)
		}
	}
}

// demoteColdDirs removes inotify watches from directories that have been inactive.
// Skips root watch dirs — those always stay hot.
func (fw *FilesystemWatcher) demoteColdDirs(timeout time.Duration) {
	now := time.Now()
	rootDirs := make(map[string]bool)
	for _, d := range fw.cfg.WatchDirs {
		rootDirs[filepath.Clean(d)] = true
	}

	fw.tierMu.Lock()
	var toDemote []string
	for dir, lastActive := range fw.hotDirs {
		// Never demote root watch dirs
		if rootDirs[filepath.Clean(dir)] {
			continue
		}
		if now.Sub(lastActive) > timeout {
			toDemote = append(toDemote, dir)
		}
	}
	fw.tierMu.Unlock()

	for _, dir := range toDemote {
		if err := fw.fsw.Remove(dir); err != nil {
			fw.log.Debug("failed to remove inotify watch for demotion", "dir", dir, "err", err)
			continue
		}

		fw.watchCount--

		fw.tierMu.Lock()
		delete(fw.hotDirs, dir)
		fw.coldDirs = append(fw.coldDirs, dir)
		if info, err := os.Stat(dir); err == nil {
			fw.coldMtimes[dir] = info.ModTime()
		}
		fw.tierMu.Unlock()

		fw.log.Info("demoted directory to cold (polling)",
			"dir", dir,
			"watch_count", fw.watchCount,
		)
	}
}

// emitPollEvent creates a filesystem event for a directory change detected by polling.
func (fw *FilesystemWatcher) emitPollEvent(dir string) {
	wevent := watcher.Event{
		ID:        uuid.New().String(),
		Source:    "filesystem",
		Type:      "dir_activity",
		Path:      dir,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"detection": "polling",
		},
	}

	select {
	case fw.events <- wevent:
		fw.log.Debug("sent polling-detected event", "dir", dir)
	case <-fw.done:
	}
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
			sensitive := len(fw.cfg.SensitivePatterns) > 0 && IsSensitiveFile(event.Name, fw.cfg.SensitivePatterns)
			fw.mu.RUnlock()
			if excluded || sensitive {
				continue
			}

			// Update hot dir activity timestamp
			dir := filepath.Dir(event.Name)
			fw.tierMu.Lock()
			if _, isHot := fw.hotDirs[dir]; isHot {
				fw.hotDirs[dir] = time.Now()
			}
			fw.tierMu.Unlock()

			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					// New directory created — add shallow watch if under budget
					if fw.watchCount < fw.maxWatches {
						if err := fw.fsw.Add(event.Name); err == nil {
							fw.watchCount++
							fw.tierMu.Lock()
							fw.hotDirs[event.Name] = time.Now()
							fw.tierMu.Unlock()
						}
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

	fw.debounce[path] = time.AfterFunc(800*time.Millisecond, func() {
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
