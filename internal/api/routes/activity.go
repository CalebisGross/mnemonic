package routes

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
)

// ActivityResponse is the JSON response for the activity endpoint.
type ActivityResponse struct {
	Concepts map[string]time.Time `json:"concepts"`
}

// HandleActivity returns the retrieval agent's current activity tracker state.
// MCP processes poll this to sync watcher-derived context boost data.
func HandleActivity(retriever *retrieval.RetrievalAgent, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snap := retriever.ActivitySnapshot()
		if snap == nil {
			snap = make(map[string]time.Time)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(ActivityResponse{Concepts: snap}); err != nil {
			log.Warn("failed to encode activity response", "error", err)
		}
	}
}
