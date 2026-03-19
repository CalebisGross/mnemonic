package routes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/google/uuid"
)

const (
	feedbackStrengthDelta float32 = 0.05
	feedbackSalienceBoost float32 = 0.02
)

// FeedbackRequest is the JSON request body for submitting recall feedback.
type FeedbackRequest struct {
	QueryID string `json:"query_id"`
	Quality string `json:"quality"` // "helpful", "partial", or "irrelevant"
}

// FeedbackResponse is the JSON response for feedback submission.
type FeedbackResponse struct {
	Status      string `json:"status"`
	Adjustments int    `json:"adjustments"`
}

// HandleFeedback returns an HTTP handler that processes recall quality feedback.
// It adjusts association strengths and memory salience based on the quality rating.
func HandleFeedback(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		defer func() { _ = r.Body.Close() }()

		var req FeedbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("failed to decode feedback request", "error", err)
			writeError(w, http.StatusBadRequest, "invalid request body", "INVALID_REQUEST")
			return
		}

		if req.QueryID == "" {
			writeError(w, http.StatusBadRequest, "query_id is required", "MISSING_FIELD")
			return
		}
		if req.Quality != "helpful" && req.Quality != "partial" && req.Quality != "irrelevant" {
			writeError(w, http.StatusBadRequest, "quality must be helpful, partial, or irrelevant", "INVALID_FIELD")
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		// Store feedback as a meta observation
		obs := store.MetaObservation{
			ID:              uuid.New().String(),
			ObservationType: "retrieval_feedback",
			Severity:        "info",
			Details: map[string]interface{}{
				"query_id": req.QueryID,
				"quality":  req.Quality,
				"source":   "web_ui",
			},
			CreatedAt: time.Now(),
		}

		if err := s.WriteMetaObservation(ctx, obs); err != nil {
			log.Error("failed to write feedback observation", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to store feedback", "STORE_ERROR")
			return
		}

		// Look up traversal data and adjust association strengths
		adjustments := 0
		fb, err := s.GetRetrievalFeedback(ctx, req.QueryID)
		if err != nil {
			log.Warn("failed to look up retrieval feedback record", "query_id", req.QueryID, "error", err)
			// Still return success — the observation was stored
			writeJSON(w, http.StatusOK, FeedbackResponse{Status: "ok", Adjustments: 0})
			return
		}

		// Update the feedback record with the quality rating
		fb.Feedback = req.Quality
		_ = s.WriteRetrievalFeedback(ctx, fb)

		switch req.Quality {
		case "helpful":
			// Strengthen traversed associations and boost returned memory salience
			for _, ta := range fb.TraversedAssocs {
				assocs, err := s.GetAssociations(ctx, ta.SourceID)
				if err != nil {
					continue
				}
				for _, a := range assocs {
					if a.TargetID == ta.TargetID {
						newStrength := a.Strength + feedbackStrengthDelta
						if newStrength > 1.0 {
							newStrength = 1.0
						}
						if err := s.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
							adjustments++
						}
						break
					}
				}
			}
			// Boost salience of returned memories
			for _, memID := range fb.RetrievedIDs {
				mem, err := s.GetMemory(ctx, memID)
				if err != nil {
					continue
				}
				newSalience := mem.Salience + feedbackSalienceBoost
				if newSalience > 1.0 {
					newSalience = 1.0
				}
				if err := s.UpdateSalience(ctx, memID, newSalience); err != nil {
					log.Warn("failed to update salience", "memory_id", memID, "error", err)
				}
			}

		case "irrelevant":
			// Weaken traversed associations
			for _, ta := range fb.TraversedAssocs {
				assocs, err := s.GetAssociations(ctx, ta.SourceID)
				if err != nil {
					continue
				}
				for _, a := range assocs {
					if a.TargetID == ta.TargetID {
						newStrength := a.Strength - feedbackStrengthDelta
						if newStrength < 0.05 {
							newStrength = 0.05
						}
						if err := s.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
							adjustments++
						}
						break
					}
				}
			}
		}

		log.Info("feedback recorded",
			"query_id", req.QueryID,
			"quality", req.Quality,
			"adjustments", adjustments)

		writeJSON(w, http.StatusOK, FeedbackResponse{
			Status:      "ok",
			Adjustments: adjustments,
		})
	}
}

// SaveRetrievalFeedback saves traversal data for a query so feedback can adjust strengths later.
// It captures a ranked access snapshot of the returned memories for metacognition analysis.
func SaveRetrievalFeedback(ctx context.Context, s store.Store, log *slog.Logger, queryID string, queryText string, results []store.RetrievalResult, traversedAssocs []store.TraversedAssoc) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var retrievedIDs []string
	var snapshot []store.AccessSnapshotEntry
	for i, r := range results {
		retrievedIDs = append(retrievedIDs, r.Memory.ID)
		snapshot = append(snapshot, store.AccessSnapshotEntry{
			MemoryID: r.Memory.ID,
			Rank:     i + 1,
			Score:    r.Score,
		})
	}

	fb := store.RetrievalFeedback{
		QueryID:         queryID,
		QueryText:       queryText,
		RetrievedIDs:    retrievedIDs,
		TraversedAssocs: traversedAssocs,
		AccessSnapshot:  snapshot,
		CreatedAt:       time.Now(),
	}
	if err := s.WriteRetrievalFeedback(ctx, fb); err != nil {
		log.Warn("failed to save retrieval feedback record", "query_id", queryID, "error", err)
	} else {
		log.Debug("saved retrieval feedback record", "query_id", queryID, "retrieved", len(retrievedIDs), "traversed", len(traversedAssocs))
	}
}
