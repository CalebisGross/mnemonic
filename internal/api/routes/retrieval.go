package routes

import (
	"log/slog"
	"net/http"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
)

// HandleRetrievalStats returns the retrieval agent's in-memory performance stats.
func HandleRetrievalStats(retriever *retrieval.RetrievalAgent, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if retriever == nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"total_queries":            0,
				"total_memories_retrieved": 0,
				"avg_memories_per_query":   0,
				"avg_synthesis_ms":         0,
			})
			return
		}
		writeJSON(w, http.StatusOK, retriever.GetStats())
	}
}
