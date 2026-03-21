package routes

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/store"
)

// AnalyticsResponse is the JSON response for the research analytics endpoint.
type AnalyticsResponse struct {
	Pipeline             PipelineMetrics                   `json:"pipeline"`
	SignalNoise          map[string]store.SignalNoiseEntry `json:"signal_noise"`
	RecallEffectiveness  []store.RecallBucket              `json:"recall_effectiveness"`
	FeedbackTrend        []store.FeedbackTrendEntry        `json:"feedback_trend"`
	ConsolidationHistory []store.ConsolidationEntry        `json:"consolidation_history"`
	MemorySurvival       []store.SurvivalEntry             `json:"memory_survival"`
	SalienceDistribution map[string]map[string]int         `json:"salience_distribution"`
	Timestamp            string                            `json:"timestamp"`
}

// PipelineMetrics shows encoding efficiency.
type PipelineMetrics struct {
	TotalRaw     int     `json:"total_raw"`
	TotalEncoded int     `json:"total_encoded"`
	TotalMerged  int     `json:"total_merged"`
	EncodingRate float64 `json:"encoding_rate"`
	DedupRate    float64 `json:"dedup_rate"`
}

// analyticsCache holds the cached response.
var (
	analyticsCache     *AnalyticsResponse
	analyticsCacheTime time.Time
	analyticsCacheMu   sync.Mutex
)

// HandleAnalytics returns research-grade metrics about the memory system.
// Results are cached for 60 seconds to avoid excessive DB queries.
func HandleAnalytics(s store.Store, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		analyticsCacheMu.Lock()
		if analyticsCache != nil && time.Since(analyticsCacheTime) < 60*time.Second {
			cached := *analyticsCache
			analyticsCacheMu.Unlock()
			cached.Timestamp = time.Now().Format(time.RFC3339)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(cached)
			return
		}
		analyticsCacheMu.Unlock()

		ctx := r.Context()
		resp := buildAnalytics(ctx, s, log)

		analyticsCacheMu.Lock()
		analyticsCache = &resp
		analyticsCacheTime = time.Now()
		analyticsCacheMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func buildAnalytics(ctx context.Context, s store.Store, log *slog.Logger) AnalyticsResponse {
	resp := AnalyticsResponse{
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Pipeline metrics
	stats, err := s.GetStatistics(ctx)
	if err != nil {
		log.Warn("analytics: stats failed", "error", err)
	} else {
		totalEncoded := stats.TotalMemories - stats.MergedMemories
		resp.Pipeline = PipelineMetrics{
			TotalEncoded: totalEncoded,
			TotalMerged:  stats.MergedMemories,
		}
		if stats.TotalMemories > 0 {
			resp.Pipeline.EncodingRate = float64(totalEncoded) / float64(stats.TotalMemories)
			resp.Pipeline.DedupRate = float64(stats.MergedMemories) / float64(stats.TotalMemories)
		}
	}

	// Use the analytics query methods
	analytics, err := s.GetAnalytics(ctx)
	if err != nil {
		log.Warn("analytics: query failed", "error", err)
		return resp
	}

	resp.Pipeline.TotalRaw = analytics.TotalRaw
	resp.SignalNoise = analytics.SignalNoise
	resp.RecallEffectiveness = analytics.RecallEffectiveness
	resp.FeedbackTrend = analytics.FeedbackTrend
	resp.ConsolidationHistory = analytics.ConsolidationHistory
	resp.MemorySurvival = analytics.MemorySurvival
	resp.SalienceDistribution = analytics.SalienceDistribution

	return resp
}
