package routes

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

// LLMUsageResponse is the JSON response for the LLM usage endpoint.
type LLMUsageResponse struct {
	Summary          store.LLMUsageSummary `json:"summary"`
	EstimatedCostUSD float64               `json:"estimated_cost_usd"`
	Log              []LLMUsageLogEntry    `json:"log"`
	Timestamp        string                `json:"timestamp"`
}

// LLMUsageLogEntry extends a usage record with estimated cost.
type LLMUsageLogEntry struct {
	llm.LLMUsageRecord
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
}

// HandleLLMUsage returns an HTTP handler that returns LLM usage summary and log.
// Query params:
//   - since: duration string (e.g. "24h", "1h", "7d") — default "24h"
//   - limit: max log entries — default 50
func HandleLLMUsage(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("llm usage requested")

		// Parse "since" duration
		sinceStr := r.URL.Query().Get("since")
		if sinceStr == "" {
			sinceStr = "24h"
		}
		sinceDur, err := time.ParseDuration(sinceStr)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid since duration: "+sinceStr, "INVALID_PARAM")
			return
		}
		since := time.Now().Add(-sinceDur)

		// Parse "limit"
		limit := parseIntParam(r, "limit", 50, 1, 1000)

		// Get summary
		summary, err := s.GetLLMUsageSummary(r.Context(), since)
		if err != nil {
			log.Error("failed to get llm usage summary", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve usage summary", "STORE_ERROR")
			return
		}

		// Get log
		records, err := s.GetLLMUsageLog(r.Context(), limit)
		if err != nil {
			log.Error("failed to get llm usage log", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve usage log", "STORE_ERROR")
			return
		}

		// Compute per-record cost and total cost
		var totalCost float64
		logEntries := make([]LLMUsageLogEntry, len(records))
		for i, rec := range records {
			cost := llm.EstimateCost(rec.Model, rec.PromptTokens, rec.CompletionTokens, 0, 0)
			totalCost += cost
			logEntries[i] = LLMUsageLogEntry{
				LLMUsageRecord:   rec,
				EstimatedCostUSD: cost,
			}
		}

		resp := LLMUsageResponse{
			Summary:          summary,
			EstimatedCostUSD: totalCost,
			Log:              logEntries,
			Timestamp:        time.Now().UTC().Format(time.RFC3339),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
