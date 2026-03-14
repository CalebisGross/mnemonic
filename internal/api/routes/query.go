package routes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// QueryRequestBody is the JSON request body for a query.
type QueryRequestBody struct {
	Query            string `json:"query"`
	Limit            int    `json:"limit,omitempty"`
	Synthesize       bool   `json:"synthesize,omitempty"`
	IncludeReasoning bool   `json:"include_reasoning,omitempty"`
}

// HandleQuery returns an HTTP handler that executes a memory retrieval query.
// Expects JSON body: {"query": "text", "limit": 7, "synthesize": true}
// Returns 200 with QueryResponse containing ranked memories and optional synthesis.
func HandleQuery(retriever *retrieval.RetrievalAgent, bus events.Bus, s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Parse request body
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		defer func() { _ = r.Body.Close() }()
		var reqBody QueryRequestBody
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			log.Warn("failed to decode query request", "error", err)
			writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		// Validate required fields
		if reqBody.Query == "" {
			writeError(w, http.StatusBadRequest, "query is required", "MISSING_FIELD")
			return
		}

		// Set defaults
		if reqBody.Limit <= 0 {
			reqBody.Limit = 7
		}
		if reqBody.Limit > 100 {
			reqBody.Limit = 100
		}

		log.Debug("executing query",
			"query", reqBody.Query,
			"limit", reqBody.Limit,
			"synthesize", reqBody.Synthesize,
			"include_reasoning", reqBody.IncludeReasoning)

		// Create retrieval request
		queryReq := retrieval.QueryRequest{
			Query:               reqBody.Query,
			MaxResults:          reqBody.Limit,
			Synthesize:          reqBody.Synthesize,
			IncludeReasoning:    reqBody.IncludeReasoning,
			IncludePatterns:     true,
			IncludeAbstractions: true,
		}

		// Execute query with timeout — must be >= LLM timeout (120s) to allow multi-turn tool-use synthesis
		ctx, cancel := context.WithTimeout(r.Context(), 180*time.Second)
		defer cancel()

		queryResp, err := retriever.Query(ctx, queryReq)
		if err != nil {
			log.Error("query execution failed", "error", err, "query", reqBody.Query)
			writeError(w, http.StatusInternalServerError, "query execution failed", "QUERY_ERROR")
			return
		}

		log.Info("query completed",
			"query_id", queryResp.QueryID,
			"results", len(queryResp.Memories),
			"took_ms", queryResp.TookMs)

		// Save traversal data for feedback loop
		var retrievedIDs []string
		for _, mem := range queryResp.Memories {
			retrievedIDs = append(retrievedIDs, mem.Memory.ID)
		}
		SaveRetrievalFeedback(ctx, s, log, queryResp.QueryID, reqBody.Query, retrievedIDs, queryResp.TraversedAssocs)

		// Publish query executed event
		queryEvt := events.QueryExecuted{
			QueryID:         queryResp.QueryID,
			QueryText:       reqBody.Query,
			ResultsReturned: len(queryResp.Memories),
			TookMs:          queryResp.TookMs,
			Ts:              time.Now(),
		}
		if err := bus.Publish(ctx, queryEvt); err != nil {
			log.Warn("failed to publish query executed event", "error", err, "query_id", queryResp.QueryID)
			// Non-critical, don't fail the request
		}

		// Publish memory accessed events for each retrieved memory
		for _, result := range queryResp.Memories {
			accessEvt := events.MemoryAccessed{
				MemoryIDs: []string{result.Memory.ID},
				QueryID:   queryResp.QueryID,
				Ts:        time.Now(),
			}
			if err := bus.Publish(ctx, accessEvt); err != nil {
				log.Warn("failed to publish memory accessed event", "error", err, "memory_id", result.Memory.ID)
			}
		}

		writeJSON(w, http.StatusOK, queryResp)
	}
}
