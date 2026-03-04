package routes

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/google/uuid"
)

// HandleGetRawMemory returns a single raw memory by ID.
// GET /api/v1/raw/{id}
func HandleGetRawMemory(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "raw memory id is required", "MISSING_ID")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		raw, err := s.GetRaw(ctx, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "raw memory not found", "NOT_FOUND")
				return
			}
			log.Error("failed to get raw memory", "error", err, "id", id)
			writeError(w, http.StatusInternalServerError, "failed to retrieve raw memory", "STORE_ERROR")
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"raw_memory": raw,
			"timestamp":  time.Now().Format(time.RFC3339),
		})
	}
}

// CreateMemoryRequest is the JSON request body for creating a memory.
type CreateMemoryRequest struct {
	Content string `json:"content"`
	Source  string `json:"source"`
	Type    string `json:"type,omitempty"`
	Project string `json:"project,omitempty"`
}

// CreateMemoryResponse is the JSON response for creating a memory.
type CreateMemoryResponse struct {
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source"`
}

// HandleCreateMemory returns an HTTP handler that creates a new raw memory.
// Expects JSON body: {"content": "text", "source": "user"}
// Returns 201 with the created memory metadata.
func HandleCreateMemory(s store.Store, bus events.Bus, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		defer r.Body.Close()
		var req CreateMemoryRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("failed to decode create memory request", "error", err)
			writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		// Validate required fields
		if req.Content == "" {
			writeError(w, http.StatusBadRequest, "content is required", "MISSING_FIELD")
			return
		}
		if req.Source == "" {
			req.Source = "web_ui"
		}
		if req.Type == "" {
			req.Type = "general"
		}

		// Create RawMemory
		now := time.Now()
		salience := float32(0.7)
		switch req.Type {
		case "decision":
			salience = 0.85
		case "error":
			salience = 0.8
		case "insight":
			salience = 0.9
		case "learning":
			salience = 0.8
		}

		rawMem := store.RawMemory{
			ID:        uuid.New().String(),
			Timestamp: now,
			Source:    req.Source,
			Type:      req.Type,
			Content:   req.Content,
			Project:   req.Project,
			Metadata: map[string]interface{}{
				"memory_type": req.Type,
				"project":     req.Project,
			},
			HeuristicScore:  0.5,
			InitialSalience: salience,
			Processed:       false,
			CreatedAt:       now,
		}

		// Write to store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := s.WriteRaw(ctx, rawMem); err != nil {
			log.Error("failed to write raw memory", "error", err, "memory_id", rawMem.ID)
			writeError(w, http.StatusInternalServerError, "failed to create memory", "STORE_ERROR")
			return
		}

		// Publish event
		evt := events.RawMemoryCreated{
			ID:             rawMem.ID,
			Source:         rawMem.Source,
			HeuristicScore: rawMem.HeuristicScore,
			Salience:       rawMem.InitialSalience,
			Ts:             now,
		}
		if err := bus.Publish(ctx, evt); err != nil {
			log.Warn("failed to publish raw memory created event", "error", err, "memory_id", rawMem.ID)
			// Don't fail the request - event publishing is non-critical
		}

		log.Info("raw memory created", "memory_id", rawMem.ID, "source", rawMem.Source)

		// Return success response
		resp := CreateMemoryResponse{
			ID:        rawMem.ID,
			Timestamp: rawMem.Timestamp,
			Source:    rawMem.Source,
		}

		writeJSON(w, http.StatusCreated, resp)
	}
}

// ListMemoriesRequest holds query parameters for listing memories.
type ListMemoriesRequest struct {
	State  string
	Limit  int
	Offset int
}

// ListMemoriesResponse is the JSON response for listing memories.
type ListMemoriesResponse struct {
	Memories []store.Memory `json:"memories"`
	Count    int            `json:"count"`
	Limit    int            `json:"limit"`
	Offset   int            `json:"offset"`
}

// HandleListMemories returns an HTTP handler that lists memories with optional filtering.
// Query params: ?state=active&limit=50&offset=0
// Returns 200 with an array of memories.
func HandleListMemories(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse query parameters
		state := r.URL.Query().Get("state")
		if state == "" {
			state = "active"
		}

		limit := 50
		if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
			if v, err := strconv.Atoi(limitStr); err == nil {
				limit = v
			}
			if limit < 1 || limit > 1000 {
				limit = 50
			}
		}

		offset := 0
		if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
			if v, err := strconv.Atoi(offsetStr); err == nil {
				offset = v
			}
			if offset < 0 {
				offset = 0
			}
		}

		log.Debug("listing memories", "state", state, "limit", limit, "offset", offset)

		// Query store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		memories, err := s.ListMemories(ctx, state, limit, offset)
		if err != nil {
			log.Error("failed to list memories", "error", err, "state", state)
			writeError(w, http.StatusInternalServerError, "failed to list memories", "STORE_ERROR")
			return
		}

		if memories == nil {
			memories = []store.Memory{}
		}

		// Filter out memories with empty summaries (e.g. from HTML file ingestion)
		filtered := make([]store.Memory, 0, len(memories))
		for _, m := range memories {
			if strings.TrimSpace(m.Summary) != "" {
				filtered = append(filtered, m)
			}
		}
		memories = filtered

		resp := ListMemoriesResponse{
			Memories: memories,
			Count:    len(memories),
			Limit:    limit,
			Offset:   offset,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// GetMemoryResponse is the JSON response for getting a single memory.
type GetMemoryResponse struct {
	Memory store.Memory `json:"memory"`
}

// HandleGetMemory returns an HTTP handler that retrieves a single memory by ID.
// URL param: {id} (parsed using Go 1.22+ r.PathValue)
// Returns 200 with the memory, or 404 if not found.
func HandleGetMemory(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from URL path (Go 1.22+)
		id := r.PathValue("id")
		if id == "" {
			writeError(w, http.StatusBadRequest, "memory id is required", "MISSING_ID")
			return
		}

		log.Debug("fetching memory", "memory_id", id)

		// Query store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		memory, err := s.GetMemory(ctx, id)
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

		resp := GetMemoryResponse{
			Memory: memory,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

// writeError writes a standard error response.
func writeError(w http.ResponseWriter, statusCode int, message string, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
		"code":  code,
	})
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}
