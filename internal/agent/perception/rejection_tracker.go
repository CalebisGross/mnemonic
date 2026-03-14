package perception

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// rejectionTracker monitors heuristic rejections by path prefix and promotes
// frequently-rejected prefixes to watcher exclusions.
type rejectionTracker struct {
	mu        sync.Mutex
	counts    map[string]int       // path prefix → rejection count
	firstSeen map[string]time.Time // path prefix → first rejection timestamp
	promoted  map[string]bool      // path prefixes already promoted

	threshold   int           // rejections required before promotion
	window      time.Duration // time window for threshold
	maxPromoted int           // cap on auto-exclusions per session

	log         *slog.Logger
	onPromote   func(pattern string) // callback when a prefix is promoted
	persistPath string               // file path for persisting learned exclusions
}

// rejectionTrackerConfig holds tunable parameters for the tracker.
type rejectionTrackerConfig struct {
	Threshold   int           // rejections to trigger promotion (default: 50)
	Window      time.Duration // time window (default: 1 hour)
	MaxPromoted int           // max auto-exclusions per session (default: 20)
	PersistPath string        // file to persist learned exclusions (empty = no persistence)
}

func newRejectionTracker(cfg rejectionTrackerConfig, log *slog.Logger, onPromote func(string)) *rejectionTracker {
	if cfg.Threshold == 0 {
		cfg.Threshold = 50
	}
	if cfg.Window == 0 {
		cfg.Window = 1 * time.Hour
	}
	if cfg.MaxPromoted == 0 {
		cfg.MaxPromoted = 20
	}

	rt := &rejectionTracker{
		counts:      make(map[string]int),
		firstSeen:   make(map[string]time.Time),
		promoted:    make(map[string]bool),
		threshold:   cfg.Threshold,
		window:      cfg.Window,
		maxPromoted: cfg.MaxPromoted,
		log:         log,
		onPromote:   onPromote,
		persistPath: cfg.PersistPath,
	}

	// Load previously learned exclusions
	if cfg.PersistPath != "" {
		rt.loadPersisted()
	}

	return rt
}

// extractPrefix extracts a 2-level directory prefix from a path under known
// base directories. For example:
//
//	/home/user/.config/Code/WebStorage/foo → .config/Code/
//	/home/user/.local/share/gnome-shell/x  → .local/share/gnome-shell/
//
// Returns empty string if no prefix can be extracted.
func extractPrefix(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	sep := string(filepath.Separator)

	// Ensure path is under home
	if !strings.HasPrefix(path, home) {
		return ""
	}

	rel := path[len(home):]
	if len(rel) > 0 && (rel[0] == '/' || rel[0] == filepath.Separator) {
		rel = rel[1:]
	}

	// Known base directories with their depth (how many levels before app dir).
	// Prefixes use filepath.Join so separators match the platform.
	bases := []struct {
		prefix string
		depth  int // number of path segments in the base prefix
	}{
		{prefix: ".config" + sep, depth: 1},
		{prefix: filepath.Join(".local", "share") + sep, depth: 2},
		{prefix: filepath.Join("Library", "Application Support") + sep, depth: 2},
		{prefix: filepath.Join("Library", "Caches") + sep, depth: 2},
	}

	for _, base := range bases {
		if strings.HasPrefix(rel, base.prefix) {
			after := rel[len(base.prefix):]
			// Get the first directory component after the base
			idx := strings.IndexByte(after, filepath.Separator)
			if idx <= 0 {
				continue
			}
			appDir := after[:idx]
			return "." + sep + rel[:len(base.prefix)+len(appDir)] + sep
		}
	}

	// Fallback: detect common project-level noise directories anywhere in the path.
	// E.g., "Projects/foo/.venv/lib/python3.12/..." → "./Projects/foo/.venv/"
	noiseDirs := []string{
		".venv" + sep, "venv" + sep, "node_modules" + sep, "__pycache__" + sep,
		"site-packages" + sep, ".tox" + sep, ".mypy_cache" + sep, ".ruff_cache" + sep, ".pytest_cache" + sep,
		".egg-info" + sep, ".eggs" + sep,
	}
	for _, noiseDir := range noiseDirs {
		idx := strings.Index(rel, noiseDir)
		if idx > 0 {
			return "." + sep + rel[:idx+len(noiseDir)]
		}
	}

	return ""
}

// recordRejection records a heuristic rejection for the given path.
// If the path prefix hits the threshold, it's promoted to an exclusion.
func (rt *rejectionTracker) recordRejection(path string) {
	prefix := extractPrefix(path)
	if prefix == "" {
		return
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Already promoted
	if rt.promoted[prefix] {
		return
	}

	// Cap on total promotions
	if len(rt.promoted) >= rt.maxPromoted {
		return
	}

	now := time.Now()

	// Initialize or check window
	if first, ok := rt.firstSeen[prefix]; ok {
		if now.Sub(first) > rt.window {
			// Window expired, reset counter
			rt.counts[prefix] = 0
			rt.firstSeen[prefix] = now
		}
	} else {
		rt.firstSeen[prefix] = now
	}

	rt.counts[prefix]++

	if rt.counts[prefix] >= rt.threshold {
		rt.promoted[prefix] = true
		rt.log.Info("auto-excluded noisy path",
			"pattern", prefix,
			"rejections", rt.counts[prefix],
			"window", rt.window,
		)

		// Persist
		if rt.persistPath != "" {
			rt.appendPersisted(prefix)
		}

		// Notify watcher
		if rt.onPromote != nil {
			rt.onPromote(prefix)
		}

		// Clean up tracking state
		delete(rt.counts, prefix)
		delete(rt.firstSeen, prefix)
	}
}

// learnedExclusions returns all exclusion patterns that have been promoted
// (both from this session and loaded from persistence).
func (rt *rejectionTracker) learnedExclusions() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	result := make([]string, 0, len(rt.promoted))
	for pattern := range rt.promoted {
		result = append(result, pattern)
	}
	return result
}

// loadPersisted reads previously learned exclusions from disk.
func (rt *rejectionTracker) loadPersisted() {
	f, err := os.Open(rt.persistPath)
	if err != nil {
		if !os.IsNotExist(err) {
			rt.log.Warn("failed to load learned exclusions", "path", rt.persistPath, "error", err)
		}
		return
	}
	defer func() { _ = f.Close() }()

	count := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rt.promoted[line] = true
		count++
	}
	if err := scanner.Err(); err != nil {
		rt.log.Warn("error reading learned exclusions", "path", rt.persistPath, "error", err)
	}
	if count > 0 {
		rt.log.Info("loaded learned exclusions", "count", count, "path", rt.persistPath)
	}
}

// appendPersisted appends a single pattern to the persistence file.
func (rt *rejectionTracker) appendPersisted(pattern string) {
	dir := filepath.Dir(rt.persistPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		rt.log.Warn("failed to create directory for learned exclusions", "error", err)
		return
	}

	f, err := os.OpenFile(rt.persistPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		rt.log.Warn("failed to persist learned exclusion", "pattern", pattern, "error", err)
		return
	}
	defer func() { _ = f.Close() }()

	if _, err := f.WriteString(pattern + "\n"); err != nil {
		rt.log.Warn("failed to write learned exclusion", "pattern", pattern, "error", err)
	}
}
