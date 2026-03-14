package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	defaultMaxSize    = 10 * 1024 * 1024 // 10MB
	defaultMaxBackups = 3
)

// Config holds logging configuration.
type Config struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// New creates a new structured logger based on configuration.
func New(cfg Config) (*slog.Logger, error) {
	level := parseLevel(cfg.Level)

	var w io.Writer

	if cfg.File != "" {
		// When a log file is configured, write only to the file.
		// This avoids duplicate log lines when running as a daemon, since
		// daemon.Start() and the LaunchAgent plist both redirect stdout to
		// the same log file — writing to both stdout and the file would
		// produce two copies of every line.
		dir := filepath.Dir(cfg.File)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("creating log directory %s: %w", dir, err)
		}
		rw, err := newRotatingWriter(cfg.File, defaultMaxSize, defaultMaxBackups)
		if err != nil {
			return nil, fmt.Errorf("opening log file %s: %w", cfg.File, err)
		}
		w = rw
	} else {
		// No file configured — write to stdout (used by CLI commands).
		w = os.Stdout
	}

	var handler slog.Handler
	opts := &slog.HandlerOptions{Level: level}

	switch strings.ToLower(cfg.Format) {
	case "json":
		handler = slog.NewJSONHandler(w, opts)
	default:
		handler = slog.NewTextHandler(w, opts)
	}

	return slog.New(handler), nil
}

// parseLevel converts a string to an slog.Level.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// rotatingWriter is an io.Writer that rotates log files when they exceed maxSize.
type rotatingWriter struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	maxSize    int64
	maxBackups int
	size       int64
}

// newRotatingWriter creates a rotating file writer.
func newRotatingWriter(path string, maxSize int64, maxBackups int) (*rotatingWriter, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	return &rotatingWriter{
		file:       f,
		path:       path,
		maxSize:    maxSize,
		maxBackups: maxBackups,
		size:       info.Size(),
	}, nil
}

// Write implements io.Writer. Rotates the file if it exceeds maxSize.
func (rw *rotatingWriter) Write(p []byte) (n int, err error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()

	if rw.size+int64(len(p)) > rw.maxSize {
		if err := rw.rotate(); err != nil {
			// If rotation fails, continue writing to the current file
			return rw.file.Write(p)
		}
	}

	n, err = rw.file.Write(p)
	rw.size += int64(n)
	return n, err
}

// rotate closes the current log file, shifts backups, and opens a new file.
func (rw *rotatingWriter) rotate() error {
	if err := rw.file.Close(); err != nil {
		return err
	}

	// Shift existing backups: .2 -> .3, .1 -> .2, etc.
	// Remove the oldest backup if it exceeds maxBackups.
	for i := rw.maxBackups; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", rw.path, i)
		dst := fmt.Sprintf("%s.%d", rw.path, i+1)
		if i == rw.maxBackups {
			_ = os.Remove(src)
		} else {
			_ = os.Rename(src, dst)
		}
	}

	// Rename current file to .1
	_ = os.Rename(rw.path, rw.path+".1")

	// Open a new file
	f, err := os.OpenFile(rw.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	rw.file = f
	rw.size = 0
	return nil
}
