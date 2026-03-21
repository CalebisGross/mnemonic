package routes

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// BackfillResponse reports what the backfill operation did.
type BackfillResponse struct {
	Total    int      `json:"total"`
	Embedded int      `json:"embedded"`
	Failed   int      `json:"failed"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// HandleBackfillEmbeddings finds memories with empty embeddings and generates them.
func HandleBackfillEmbeddings(s store.Store, provider llm.Provider, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		// Find all active memories missing embeddings
		memories, err := s.ListMemories(ctx, "", 500, 0)
		if err != nil {
			log.Error("backfill: failed to list memories", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list memories", "STORE_ERROR")
			return
		}

		var missing []store.Memory
		for _, m := range memories {
			if len(m.Embedding) == 0 {
				missing = append(missing, m)
			}
		}

		if len(missing) == 0 {
			writeJSON(w, http.StatusOK, BackfillResponse{Total: 0})
			return
		}

		log.Info("backfill: starting embedding backfill", "missing", len(missing))

		// Quick sanity check: can we embed at all?
		testEmb, testErr := provider.Embed(ctx, "test embedding sanity check")
		if testErr != nil {
			log.Error("backfill: embedding sanity check failed", "error", testErr)
			writeJSON(w, http.StatusOK, BackfillResponse{Total: len(missing), Errors: []string{"sanity check failed: " + testErr.Error()}})
			return
		}
		log.Info("backfill: sanity check passed", "dims", len(testEmb))

		resp := BackfillResponse{Total: len(missing)}

		for _, mem := range missing {
			select {
			case <-ctx.Done():
				log.Warn("backfill: context cancelled", "embedded", resp.Embedded, "remaining", resp.Total-resp.Embedded-resp.Failed)
				writeJSON(w, http.StatusOK, resp)
				return
			default:
			}

			// Build embedding text from summary + content (same as encoding agent)
			text := mem.Summary + " " + mem.Content
			if len(text) > 4000 {
				text = text[:4000]
			}

			embedding, err := provider.Embed(ctx, text)
			if err != nil {
				resp.Errors = append(resp.Errors, "embed:"+mem.ID[:8]+":"+err.Error())
				resp.Failed++
				continue
			}

			if len(embedding) == 0 {
				resp.Skipped++
				continue
			}

			// Use targeted update to avoid FK issues with raw_id
			if err := s.UpdateEmbedding(ctx, mem.ID, embedding); err != nil {
				resp.Errors = append(resp.Errors, "update:"+mem.ID[:8]+":"+err.Error())
				resp.Failed++
				continue
			}

			resp.Embedded++
			log.Debug("backfill: embedded memory", "id", mem.ID, "dims", len(embedding))
		}

		log.Info("backfill: completed", "total", resp.Total, "embedded", resp.Embedded, "failed", resp.Failed)
		writeJSON(w, http.StatusOK, resp)
	}
}
