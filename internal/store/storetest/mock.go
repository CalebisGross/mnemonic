// Package storetest provides a shared base mock implementation of store.Store
// for use in tests. Embed MockStore in your test-local mockStore struct and
// override only the methods your tests need.
package storetest

import (
	"context"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
)

// MockStore implements every method of store.Store with zero-value returns.
// Embed it in test-local mock structs and override only what you need.
type MockStore struct{}

// Compile-time interface check.
var _ store.Store = MockStore{}

// --- Raw memory operations ---

func (MockStore) WriteRaw(context.Context, store.RawMemory) error { return nil }
func (MockStore) GetRaw(context.Context, string) (store.RawMemory, error) {
	return store.RawMemory{}, nil
}
func (MockStore) ListRawUnprocessed(context.Context, int) ([]store.RawMemory, error) {
	return nil, nil
}
func (MockStore) ListRawMemoriesAfter(context.Context, time.Time, int) ([]store.RawMemory, error) {
	return nil, nil
}
func (MockStore) MarkRawProcessed(context.Context, string) error    { return nil }
func (MockStore) ClaimRawForEncoding(context.Context, string) error { return nil }
func (MockStore) UnclaimRawMemory(context.Context, string) error    { return nil }

// --- Encoded memory operations ---

func (MockStore) WriteMemory(context.Context, store.Memory) error { return nil }
func (MockStore) GetMemory(context.Context, string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (MockStore) GetMemoryByRawID(context.Context, string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (MockStore) UpdateMemory(context.Context, store.Memory) error      { return nil }
func (MockStore) UpdateSalience(context.Context, string, float32) error { return nil }
func (MockStore) UpdateState(context.Context, string, string) error     { return nil }
func (MockStore) IncrementAccess(context.Context, string) error         { return nil }
func (MockStore) ListMemories(context.Context, string, int, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) CountMemories(context.Context) (int, error) { return 0, nil }

// --- Memory amendment ---

func (MockStore) AmendMemory(context.Context, string, string, string, []string, []float32) error {
	return nil
}

// --- Search operations ---

func (MockStore) SearchByFullText(context.Context, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) SearchByEmbedding(context.Context, []float32, int) ([]store.RetrievalResult, error) {
	return nil, nil
}
func (MockStore) SearchByConcepts(context.Context, []string, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) SearchByConceptsInProject(context.Context, []string, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) GetAnalytics(context.Context) (store.AnalyticsData, error) {
	return store.AnalyticsData{}, nil
}
func (MockStore) RawMemoryExistsByHash(context.Context, string) (bool, error) {
	return false, nil
}

// --- Association graph operations ---

func (MockStore) CreateAssociation(context.Context, store.Association) error { return nil }
func (MockStore) GetAssociations(context.Context, string) ([]store.Association, error) {
	return nil, nil
}
func (MockStore) UpdateAssociationStrength(context.Context, string, string, float32) error {
	return nil
}
func (MockStore) UpdateAssociationType(context.Context, string, string, string) error {
	return nil
}
func (MockStore) ActivateAssociation(context.Context, string, string) error { return nil }
func (MockStore) PruneWeakAssociations(context.Context, float32) (int, error) {
	return 0, nil
}
func (MockStore) PruneOrphanedAssociations(context.Context) (int, error) {
	return 0, nil
}

// --- Deduplication ---

func (MockStore) RawMemoryExistsByPath(context.Context, string, string, string) (bool, error) {
	return false, nil
}

// --- Cleanup operations ---

func (MockStore) CountRawUnprocessedByPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}
func (MockStore) BulkMarkRawProcessedByPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}
func (MockStore) ArchiveMemoriesByRawPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}

// --- Batch operations ---

func (MockStore) BatchWriteRaw(context.Context, []store.RawMemory) error        { return nil }
func (MockStore) BatchUpdateSalience(context.Context, map[string]float32) error { return nil }
func (MockStore) BatchMergeMemories(context.Context, []string, store.Memory) error {
	return nil
}
func (MockStore) DeleteOldArchived(context.Context, time.Time) (int, error) { return 0, nil }

// --- Consolidation tracking ---

func (MockStore) WriteConsolidation(context.Context, store.ConsolidationRecord) error {
	return nil
}
func (MockStore) GetLastConsolidation(context.Context) (store.ConsolidationRecord, error) {
	return store.ConsolidationRecord{}, nil
}

// --- Export/Backup operations ---

func (MockStore) ListAllAssociations(context.Context) ([]store.Association, error) {
	return nil, nil
}
func (MockStore) ListAllRawMemories(context.Context) ([]store.RawMemory, error) {
	return nil, nil
}

// --- Scoped association queries ---

func (MockStore) GetAssociationsForMemoryIDs(context.Context, []string) ([]store.Association, error) {
	return nil, nil
}

// --- Metacognition operations ---

func (MockStore) WriteMetaObservation(context.Context, store.MetaObservation) error { return nil }
func (MockStore) ListMetaObservations(context.Context, string, int) ([]store.MetaObservation, error) {
	return nil, nil
}
func (MockStore) DeleteOldMetaObservations(context.Context, time.Time) (int, error) {
	return 0, nil
}
func (MockStore) GetDeadMemories(context.Context, time.Time) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) GetSourceDistribution(context.Context) (map[string]int, error) {
	return nil, nil
}

// --- Retrieval feedback operations ---

func (MockStore) WriteRetrievalFeedback(context.Context, store.RetrievalFeedback) error {
	return nil
}
func (MockStore) GetRetrievalFeedback(context.Context, string) (store.RetrievalFeedback, error) {
	return store.RetrievalFeedback{}, nil
}
func (MockStore) ListRecentRetrievalFeedback(context.Context, time.Time, int) ([]store.RetrievalFeedback, error) {
	return nil, nil
}
func (MockStore) GetMemoryFeedbackScores(context.Context, []string) (map[string]float32, error) {
	return nil, nil
}

// --- Episode operations ---

func (MockStore) CreateEpisode(context.Context, store.Episode) error { return nil }
func (MockStore) GetEpisode(context.Context, string) (store.Episode, error) {
	return store.Episode{}, nil
}
func (MockStore) UpdateEpisode(context.Context, store.Episode) error { return nil }
func (MockStore) ListEpisodes(context.Context, string, int, int) ([]store.Episode, error) {
	return nil, nil
}
func (MockStore) GetOpenEpisode(context.Context) (store.Episode, error) {
	return store.Episode{}, nil
}
func (MockStore) CloseEpisode(context.Context, string) error { return nil }

// --- Multi-resolution operations ---

func (MockStore) WriteMemoryResolution(context.Context, store.MemoryResolution) error {
	return nil
}
func (MockStore) GetMemoryResolution(context.Context, string) (store.MemoryResolution, error) {
	return store.MemoryResolution{}, nil
}

// --- Structured concept operations ---

func (MockStore) WriteConceptSet(context.Context, store.ConceptSet) error { return nil }
func (MockStore) GetConceptSet(context.Context, string) (store.ConceptSet, error) {
	return store.ConceptSet{}, nil
}
func (MockStore) SearchByEntity(context.Context, string, string, int) ([]store.Memory, error) {
	return nil, nil
}

// --- Memory attribute operations ---

func (MockStore) WriteMemoryAttributes(context.Context, store.MemoryAttributes) error {
	return nil
}
func (MockStore) GetMemoryAttributes(context.Context, string) (store.MemoryAttributes, error) {
	return store.MemoryAttributes{}, nil
}

// --- Pattern operations ---

func (MockStore) WritePattern(context.Context, store.Pattern) error { return nil }
func (MockStore) GetPattern(context.Context, string) (store.Pattern, error) {
	return store.Pattern{}, nil
}
func (MockStore) UpdatePattern(context.Context, store.Pattern) error { return nil }
func (MockStore) ListPatterns(context.Context, string, int) ([]store.Pattern, error) {
	return nil, nil
}
func (MockStore) SearchPatternsByEmbedding(context.Context, []float32, int) ([]store.Pattern, error) {
	return nil, nil
}
func (MockStore) ArchiveAllPatterns(context.Context) (int, error) { return 0, nil }

// --- Abstraction operations ---

func (MockStore) WriteAbstraction(context.Context, store.Abstraction) error { return nil }
func (MockStore) GetAbstraction(context.Context, string) (store.Abstraction, error) {
	return store.Abstraction{}, nil
}
func (MockStore) UpdateAbstraction(context.Context, store.Abstraction) error { return nil }
func (MockStore) ListAbstractions(context.Context, int, int) ([]store.Abstraction, error) {
	return nil, nil
}
func (MockStore) ListAbstractionsByState(context.Context, string, int) ([]store.Abstraction, error) {
	return nil, nil
}
func (MockStore) SearchAbstractionsByEmbedding(context.Context, []float32, int) ([]store.Abstraction, error) {
	return nil, nil
}
func (MockStore) ArchiveAllAbstractions(context.Context) (int, error) { return 0, nil }

// --- Scoped queries ---

func (MockStore) SearchByProject(context.Context, string, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) ListMemoriesByTimeRange(context.Context, time.Time, time.Time, int) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) ListMemoriesBySession(context.Context, string) ([]store.Memory, error) {
	return nil, nil
}
func (MockStore) GetProjectSummary(context.Context, string) (map[string]interface{}, error) {
	return nil, nil
}
func (MockStore) ListProjects(context.Context) ([]string, error) { return nil, nil }

// --- Runtime exclusions ---

func (MockStore) AddRuntimeExclusion(context.Context, string) error    { return nil }
func (MockStore) RemoveRuntimeExclusion(context.Context, string) error { return nil }
func (MockStore) ListRuntimeExclusions(context.Context) ([]string, error) {
	return nil, nil
}

// --- Session queries ---

func (MockStore) ListSessions(context.Context, time.Time, int) ([]store.SessionSummary, error) {
	return nil, nil
}
func (MockStore) GetSessionMemories(context.Context, string, int) ([]store.Memory, error) {
	return nil, nil
}

// --- Housekeeping ---

func (MockStore) GetStatistics(context.Context) (store.StoreStatistics, error) {
	return store.StoreStatistics{}, nil
}

// --- LLM usage tracking ---

func (MockStore) RecordLLMUsage(context.Context, llm.LLMUsageRecord) error { return nil }
func (MockStore) GetLLMUsageSummary(context.Context, time.Time) (store.LLMUsageSummary, error) {
	return store.LLMUsageSummary{}, nil
}
func (MockStore) GetLLMUsageLog(context.Context, time.Time, int) ([]llm.LLMUsageRecord, error) {
	return nil, nil
}
func (MockStore) GetLLMUsageChart(context.Context, time.Time, int) ([]store.LLMChartBucket, error) {
	return nil, nil
}

// --- MCP tool usage tracking ---

func (MockStore) RecordToolUsage(context.Context, store.ToolUsageRecord) error { return nil }
func (MockStore) GetToolUsageSummary(context.Context, time.Time) (store.ToolUsageSummary, error) {
	return store.ToolUsageSummary{}, nil
}
func (MockStore) GetToolUsageLog(context.Context, time.Time, int) ([]store.ToolUsageRecord, error) {
	return nil, nil
}
func (MockStore) GetToolUsageChart(context.Context, time.Time, int) ([]store.ToolChartBucket, error) {
	return nil, nil
}

// --- Lifecycle ---

func (MockStore) Close() error { return nil }
