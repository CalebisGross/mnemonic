package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/ingest"
	"github.com/appsprout/mnemonic/internal/store"
)

// IngestRequest is the JSON request body for the ingest endpoint.
type IngestRequest struct {
	Directory string `json:"directory"`
	Project   string `json:"project"`
	DryRun    bool   `json:"dry_run"`
}

// IngestResponse is the JSON response for the ingest endpoint.
type IngestResponse struct {
	FilesFound        int    `json:"files_found"`
	FilesWritten      int    `json:"files_written"`
	FilesSkipped      int    `json:"files_skipped"`
	FilesFailed       int    `json:"files_failed"`
	DuplicatesSkipped int    `json:"duplicates_skipped"`
	Project           string `json:"project"`
	ElapsedMs         int64  `json:"elapsed_ms"`
}

// HandleIngest returns an HTTP handler that ingests a directory into the memory system.
func HandleIngest(s store.Store, bus events.Bus, excludePatterns []string, maxContentBytes int, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer r.Body.Close()

		var req IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("failed to decode ingest request", "error", err)
			writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		if req.Directory == "" {
			writeError(w, http.StatusBadRequest, "directory is required", "MISSING_FIELD")
			return
		}

		cfg := ingest.Config{
			Dir:             req.Directory,
			Project:         req.Project,
			DryRun:          req.DryRun,
			ExcludePatterns: excludePatterns,
			MaxContentBytes: maxContentBytes,
		}

		result, err := ingest.Run(r.Context(), cfg, s, bus, log)
		if err != nil {
			log.Error("ingest failed", "directory", req.Directory, "error", err)
			writeError(w, http.StatusInternalServerError, err.Error(), "INGEST_ERROR")
			return
		}

		log.Info("ingest completed",
			"directory", req.Directory,
			"project", result.Project,
			"files_written", result.FilesWritten,
			"duplicates_skipped", result.DuplicatesSkipped)

		writeJSON(w, http.StatusOK, IngestResponse{
			FilesFound:        result.FilesFound,
			FilesWritten:      result.FilesWritten,
			FilesSkipped:      result.FilesSkipped,
			FilesFailed:       result.FilesFailed,
			DuplicatesSkipped: result.DuplicatesSkipped,
			Project:           result.Project,
			ElapsedMs:         result.Elapsed.Milliseconds(),
		})
	}
}
