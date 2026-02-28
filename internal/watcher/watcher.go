package watcher

import (
	"context"
	"time"
)

// Event represents a raw observation from a watcher.
type Event struct {
	ID        string                 `json:"id"`
	Source    string                 `json:"source"` // "filesystem", "terminal", "clipboard"
	Type      string                 `json:"type"`   // "file_created", "file_modified", "command_executed", "clipboard_changed"
	Timestamp time.Time              `json:"timestamp"`
	Path      string                 `json:"path,omitempty"` // for filesystem events
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// Watcher is the abstraction for a perception source.
type Watcher interface {
	// Name returns the watcher's identifier.
	Name() string

	// Start begins watching. Should launch goroutines and return immediately.
	Start(ctx context.Context) error

	// Stop gracefully stops the watcher.
	Stop() error

	// Events returns a channel of observed events.
	Events() <-chan Event

	// Health checks if the watcher is functioning.
	Health(ctx context.Context) error
}
