package routes

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

// ToolUsageResponse is the JSON response for the tool usage endpoint.
type ToolUsageResponse struct {
	Summary      store.ToolUsageSummary  `json:"summary"`
	Log          []store.ToolUsageRecord `json:"log"`
	ChartBuckets []store.ToolChartBucket `json:"chart_buckets"`
	Timestamp    string                  `json:"timestamp"`
}

// HandleToolUsage returns an HTTP handler that returns MCP tool usage summary and log.
// Query params:
//   - since: duration string (e.g. "24h", "1h", "7d") — default "24h"
//   - limit: max log entries — default 50
func HandleToolUsage(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debug("tool usage requested")

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
		summary, err := s.GetToolUsageSummary(r.Context(), since)
		if err != nil {
			log.Error("failed to get tool usage summary", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve tool usage summary", "STORE_ERROR")
			return
		}

		// Get log
		records, err := s.GetToolUsageLog(r.Context(), since, limit)
		if err != nil {
			log.Error("failed to get tool usage log", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to retrieve tool usage log", "STORE_ERROR")
			return
		}

		// Get chart data
		bSecs := bucketSeconds[sinceStr]
		if bSecs == 0 {
			bSecs = 3600
		}
		chartBuckets, err := s.GetToolUsageChart(r.Context(), since, bSecs)
		if err != nil {
			log.Error("failed to get tool chart data", "error", err)
			chartBuckets = nil
		}

		resp := ToolUsageResponse{
			Summary:      summary,
			Log:          records,
			ChartBuckets: chartBuckets,
			Timestamp:    time.Now().UTC().Format(time.RFC3339),
		}

		writeJSON(w, http.StatusOK, resp)
	}
}
