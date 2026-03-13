package routes

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout/mnemonic/internal/store"
)

// HandleListEpisodes returns episodes with pagination.
// GET /api/v1/episodes?state=&limit=50&offset=0
func HandleListEpisodes(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state := r.URL.Query().Get("state")
		limit := parseIntParam(r, "limit", 50, 1, 200)
		offset := parseIntParam(r, "offset", 0, 0, 100000)

		log.Debug("listing episodes", "state", state, "limit", limit, "offset", offset)

		// Query store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		episodes, err := s.ListEpisodes(ctx, state, limit, offset)
		if err != nil {
			log.Error("failed to list episodes", "error", err, "state", state)
			writeError(w, http.StatusInternalServerError, "failed to list episodes", "STORE_ERROR")
			return
		}

		if episodes == nil {
			episodes = []store.Episode{}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"episodes":  episodes,
			"count":     len(episodes),
			"limit":     limit,
			"offset":    offset,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}

// HandleGetEpisode returns a single episode with its memories.
// GET /api/v1/episodes/{id}
func HandleGetEpisode(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "episode id is required", "MISSING_ID")
			return
		}

		log.Debug("fetching episode", "episode_id", id)

		// Query store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		episode, err := s.GetEpisode(ctx, id)
		if err != nil {
			// Check if it's a not-found error
			if errors.Is(err, store.ErrNotFound) {
				log.Debug("episode not found", "episode_id", id)
				writeError(w, http.StatusNotFound, "episode not found", "NOT_FOUND")
				return
			}

			log.Error("failed to get episode", "error", err, "episode_id", id)
			writeError(w, http.StatusInternalServerError, "failed to retrieve episode", "STORE_ERROR")
			return
		}

		// Fetch memories for this episode
		var memories []store.Memory
		for _, memID := range episode.MemoryIDs {
			mem, err := s.GetMemory(ctx, memID)
			if err == nil {
				memories = append(memories, mem)
			}
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"episode":   episode,
			"memories":  memories,
			"timestamp": time.Now().Format(time.RFC3339),
		})
	}
}

// HandleMemoryContext returns rich context for a single memory.
// GET /api/v1/memories/{id}/context
func HandleMemoryContext(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "memory id is required", "MISSING_ID")
			return
		}

		log.Debug("fetching memory context", "memory_id", id)

		// Query store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		mem, err := s.GetMemory(ctx, id)
		if err != nil {
			// Check if it's a not-found error
			if errors.Is(err, store.ErrNotFound) {
				log.Debug("memory not found", "memory_id", id)
				writeError(w, http.StatusNotFound, "memory not found", "NOT_FOUND")
				return
			}

			log.Error("failed to get memory", "error", err, "memory_id", id)
			writeError(w, http.StatusInternalServerError, "failed to retrieve memory", "STORE_ERROR")
			return
		}

		response := map[string]interface{}{
			"memory":    mem,
			"timestamp": time.Now().Format(time.RFC3339),
		}

		// Get resolution (gist + narrative)
		res, err := s.GetMemoryResolution(ctx, id)
		if err == nil {
			response["resolution"] = res
		}

		// Get structured concepts
		cs, err := s.GetConceptSet(ctx, id)
		if err == nil {
			response["concept_set"] = cs
		}

		// Get attributes (valence)
		attrs, err := s.GetMemoryAttributes(ctx, id)
		if err == nil {
			response["attributes"] = attrs
		}

		// Get episode info
		if mem.EpisodeID != "" {
			ep, err := s.GetEpisode(ctx, mem.EpisodeID)
			if err == nil {
				response["episode"] = ep
			}
		}

		writeJSON(w, http.StatusOK, response)
	}
}
