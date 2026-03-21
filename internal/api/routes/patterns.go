package routes

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

// HandleListPatterns returns discovered patterns.
// GET /api/v1/patterns?project=&limit=20
func HandleListPatterns(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		project := r.URL.Query().Get("project")
		limit := parseIntParam(r, "limit", 20, 1, 100)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		patterns, err := s.ListPatterns(ctx, project, limit)
		if err != nil {
			log.Error("failed to list patterns", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list patterns", "STORE_ERROR")
			return
		}
		if patterns == nil {
			patterns = []store.Pattern{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"patterns":  patterns,
			"count":     len(patterns),
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}

// HandleListAbstractions returns hierarchical abstractions.
// GET /api/v1/abstractions?level=0&limit=20
func HandleListAbstractions(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		level := parseIntParam(r, "level", 0, 0, 10)
		limit := parseIntParam(r, "limit", 20, 1, 100)

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var abstractions []store.Abstraction
		if level > 0 {
			abs, err := s.ListAbstractions(ctx, level, limit)
			if err != nil {
				log.Error("failed to list abstractions", "error", err, "level", level)
				writeError(w, http.StatusInternalServerError, "failed to list abstractions", "STORE_ERROR")
				return
			}
			abstractions = abs
		} else {
			// Fetch all levels
			for _, lvl := range []int{2, 3} {
				abs, err := s.ListAbstractions(ctx, lvl, limit)
				if err != nil {
					continue
				}
				abstractions = append(abstractions, abs...)
			}
		}

		if abstractions == nil {
			abstractions = []store.Abstraction{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"abstractions": abstractions,
			"count":        len(abstractions),
			"timestamp":    time.Now().Format(time.RFC3339),
		})
	}
}

// HandleArchivePattern archives a single pattern by ID.
// PATCH /api/v1/patterns/{id}  body: {"state": "archived"}
func HandleArchivePattern(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "pattern ID is required", "INVALID_REQUEST")
			return
		}

		var body struct {
			State string `json:"state"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.State == "" {
			writeError(w, http.StatusBadRequest, "state field is required (e.g. \"archived\")", "INVALID_REQUEST")
			return
		}
		if body.State != "active" && body.State != "archived" {
			writeError(w, http.StatusBadRequest, "state must be \"active\" or \"archived\"", "INVALID_REQUEST")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		p, err := s.GetPattern(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "pattern not found", "NOT_FOUND")
				return
			}
			log.Error("failed to get pattern", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to get pattern", "STORE_ERROR")
			return
		}

		p.State = body.State
		p.UpdatedAt = time.Now()
		if err := s.UpdatePattern(ctx, p); err != nil {
			log.Error("failed to update pattern", "id", id, "error", err)
			writeError(w, http.StatusInternalServerError, "failed to update pattern", "STORE_ERROR")
			return
		}

		log.Info("pattern state updated", "id", id, "state", body.State)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"id":    id,
			"state": body.State,
		})
	}
}

// HandleListProjects returns all known projects.
// GET /api/v1/projects
func HandleListProjects(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		projects, err := s.ListProjects(ctx)
		if err != nil {
			log.Error("failed to list projects", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list projects", "STORE_ERROR")
			return
		}
		if projects == nil {
			projects = []string{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"projects":  projects,
			"count":     len(projects),
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}
