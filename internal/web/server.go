package web

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

// RegisterRoutes registers the web UI routes on the given ServeMux.
// Serves the embedded static files at the root path.
// Returns an error if the embedded filesystem cannot be loaded.
func RegisterRoutes(mux *http.ServeMux) error {
	// Create a sub-filesystem rooted at "static"
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return fmt.Errorf("creating static filesystem: %w", err)
	}

	// Serve static files
	fileServer := http.FileServer(http.FS(staticFS))

	// Handle root path - serve index.html
	mux.Handle("/", fileServer)
	return nil
}
