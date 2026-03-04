package routes

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

// HealthResponse is the JSON response for the health check endpoint.
type HealthResponse struct {
	Status       string `json:"status"`
	LLMAvailable bool   `json:"llm_available"`
	LLMModel     string `json:"llm_model,omitempty"`
	StoreHealthy bool   `json:"store_healthy"`
	MemoryCount  int    `json:"memory_count"`
	Timestamp    string `json:"timestamp"`
}

// HandleHealth returns an HTTP handler that performs a health check.
// Checks LLM availability with 2s timeout and store health.
// Returns 200 with health status JSON.
func HandleHealth(s store.Store, llmProv llm.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("health check requested")

		// Check LLM health with 2s timeout
		llmHealthCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		llmAvailable := true
		var llmModel string
		if err := llmProv.Health(llmHealthCtx); err != nil {
			log.Warn("llm health check failed", "error", err)
			llmAvailable = false
		} else {
			if info, err := llmProv.ModelInfo(llmHealthCtx); err == nil {
				llmModel = info.Name
			}
		}

		// Check store health by counting memories
		storeHealthy := true
		var memoryCount int
		storeCtx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		count, err := s.CountMemories(storeCtx)
		if err != nil {
			log.Warn("store health check failed", "error", err)
			storeHealthy = false
			memoryCount = 0
		} else {
			memoryCount = count
		}

		status := "ok"
		if !llmAvailable || !storeHealthy {
			status = "degraded"
		}

		resp := HealthResponse{
			Status:       status,
			LLMAvailable: llmAvailable,
			LLMModel:     llmModel,
			StoreHealthy: storeHealthy,
			MemoryCount:  memoryCount,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
		}

		log.Info("health check completed", "status", status, "llm_available", llmAvailable, "store_healthy", storeHealthy, "memory_count", memoryCount)

		writeJSON(w, http.StatusOK, resp)
	}
}

// StatsResponse is the JSON response for the stats endpoint.
type StatsResponse struct {
	Store     store.StoreStatistics `json:"store"`
	Timestamp string                `json:"timestamp"`
}

// HandleStats returns an HTTP handler that returns system statistics.
// Returns 200 with StoreStatistics JSON.
func HandleStats(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("stats requested")

		// Get statistics from store
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		stats, err := s.GetStatistics(ctx)
		if err != nil {
			log.Error("failed to get statistics", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve statistics", "STORE_ERROR")
			return
		}

		resp := StatsResponse{
			Store:     stats,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		}

		log.Debug("stats retrieved", "total_memories", stats.TotalMemories, "active", stats.ActiveMemories)

		writeJSON(w, http.StatusOK, resp)
	}
}

// ConsolidationRunResponse is the JSON response for the consolidation run endpoint.
type ConsolidationRunResponse struct {
	Status             string `json:"status"`
	DurationMs         int64  `json:"duration_ms,omitempty"`
	MemoriesProcessed  int    `json:"memories_processed,omitempty"`
	MemoriesDecayed    int    `json:"memories_decayed,omitempty"`
	TransitionedFading int    `json:"transitioned_fading,omitempty"`
	AssociationsPruned int    `json:"associations_pruned,omitempty"`
	MergesPerformed    int    `json:"merges_performed,omitempty"`
	Note               string `json:"note,omitempty"`
}

// ConsolidationRunner is the interface for triggering consolidation from the API.
// RunConsolidation runs a single consolidation cycle and returns an error if it fails.
type ConsolidationRunner interface {
	RunConsolidation(ctx context.Context) error
}

// HandleConsolidationRun returns an HTTP handler for triggering consolidation.
func HandleConsolidationRun(runner ConsolidationRunner, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Info("consolidation run triggered via API")

		if runner == nil {
			resp := ConsolidationRunResponse{
				Status: "disabled",
				Note:   "Consolidation agent is not enabled in config",
			}
			writeJSON(w, http.StatusOK, resp)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		err := runner.RunConsolidation(ctx)
		if err != nil {
			log.Error("consolidation run failed", "error", err)
			writeError(w, http.StatusInternalServerError, "consolidation failed: "+err.Error(), "CONSOLIDATION_ERROR")
			return
		}

		resp := ConsolidationRunResponse{
			Status: "completed",
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

// InsightsResponse is the JSON response for the insights endpoint.
type InsightsResponse struct {
	Observations []store.MetaObservation `json:"observations"`
	Timestamp    string                  `json:"timestamp"`
}

// HandleInsights returns an HTTP handler that returns metacognition insights.
func HandleInsights(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("insights requested")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		observations, err := s.ListMetaObservations(ctx, "", 20)
		if err != nil {
			log.Error("failed to get meta observations", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve insights", "STORE_ERROR")
			return
		}

		resp := InsightsResponse{
			Observations: observations,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
