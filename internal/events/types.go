package events

import "time"

// --- Event type constants ---
const (
	TypeRawMemoryCreated       = "raw_memory_created"
	TypeMemoryEncoded          = "memory_encoded"
	TypeMemoryAccessed         = "memory_accessed"
	TypeConsolidationStarted   = "consolidation_started"
	TypeConsolidationCompleted = "consolidation_completed"
	TypeQueryExecuted          = "query_executed"
	TypeMetaCycleCompleted     = "meta_cycle_completed"
	TypeDreamCycleCompleted    = "dream_cycle_completed"
	TypeSystemHealth           = "system_health"
	TypeWatcherEvent           = "watcher_event"
	TypeEpisodeClosed          = "episode_closed"
	TypePatternDiscovered      = "pattern_discovered"
)

// RawMemoryCreated is emitted when a raw memory is ingested.
type RawMemoryCreated struct {
	ID             string    `json:"id"`
	Source         string    `json:"source"`
	HeuristicScore float32   `json:"heuristic_score"`
	Salience       float32   `json:"salience"`
	Ts             time.Time `json:"timestamp"`
}

func (e RawMemoryCreated) EventType() string         { return TypeRawMemoryCreated }
func (e RawMemoryCreated) EventTimestamp() time.Time { return e.Ts }

// MemoryEncoded is emitted when a raw memory has been encoded and stored.
type MemoryEncoded struct {
	MemoryID            string    `json:"memory_id"`
	RawID               string    `json:"raw_id"`
	Concepts            []string  `json:"concepts"`
	AssociationsCreated int       `json:"associations_created"`
	Ts                  time.Time `json:"timestamp"`
}

func (e MemoryEncoded) EventType() string         { return TypeMemoryEncoded }
func (e MemoryEncoded) EventTimestamp() time.Time { return e.Ts }

// MemoryAccessed is emitted when memories are retrieved.
type MemoryAccessed struct {
	MemoryIDs []string  `json:"memory_ids"`
	QueryID   string    `json:"query_id"`
	Ts        time.Time `json:"timestamp"`
}

func (e MemoryAccessed) EventType() string         { return TypeMemoryAccessed }
func (e MemoryAccessed) EventTimestamp() time.Time { return e.Ts }

// ConsolidationStarted is emitted when a consolidation cycle begins.
type ConsolidationStarted struct {
	Ts time.Time `json:"timestamp"`
}

func (e ConsolidationStarted) EventType() string         { return TypeConsolidationStarted }
func (e ConsolidationStarted) EventTimestamp() time.Time { return e.Ts }

// ConsolidationCompleted is emitted when a consolidation cycle finishes.
type ConsolidationCompleted struct {
	DurationMs            int64     `json:"duration_ms"`
	MemoriesProcessed     int       `json:"memories_processed"`
	MemoriesDecayed       int       `json:"memories_decayed"`
	MergedClusters        int       `json:"merged_clusters"`
	AssociationsPruned    int       `json:"associations_pruned"`
	TransitionedFading    int       `json:"transitioned_fading"`
	TransitionedArchived  int       `json:"transitioned_archived"`
	PatternsExtracted     int       `json:"patterns_extracted"`
	PatternsDecayed       int       `json:"patterns_decayed"`
	NeverRecalledArchived int       `json:"never_recalled_archived"`
	Ts                    time.Time `json:"timestamp"`
}

func (e ConsolidationCompleted) EventType() string         { return TypeConsolidationCompleted }
func (e ConsolidationCompleted) EventTimestamp() time.Time { return e.Ts }

// QueryExecuted is emitted when a query is processed.
type QueryExecuted struct {
	QueryID         string    `json:"query_id"`
	QueryText       string    `json:"query_text"`
	ResultsReturned int       `json:"results_returned"`
	TookMs          int64     `json:"took_ms"`
	Ts              time.Time `json:"timestamp"`
}

func (e QueryExecuted) EventType() string         { return TypeQueryExecuted }
func (e QueryExecuted) EventTimestamp() time.Time { return e.Ts }

// MetaCycleCompleted is emitted when meta-cognition completes a monitoring cycle.
type MetaCycleCompleted struct {
	ObservationsLogged int       `json:"observations_logged"`
	Ts                 time.Time `json:"timestamp"`
}

func (e MetaCycleCompleted) EventType() string         { return TypeMetaCycleCompleted }
func (e MetaCycleCompleted) EventTimestamp() time.Time { return e.Ts }

// SystemHealth is emitted periodically with system status.
type SystemHealth struct {
	LLMAvailable   bool      `json:"llm_available"`
	StoreHealthy   bool      `json:"store_healthy"`
	ActiveWatchers int       `json:"active_watchers"`
	MemoryCount    int       `json:"memory_count"`
	Ts             time.Time `json:"timestamp"`
}

func (e SystemHealth) EventType() string         { return TypeSystemHealth }
func (e SystemHealth) EventTimestamp() time.Time { return e.Ts }

// WatcherEvent is emitted when a watcher observes something.
type WatcherEvent struct {
	Source  string    `json:"source"`
	Type    string    `json:"type"`
	Path    string    `json:"path,omitempty"`
	Preview string    `json:"preview,omitempty"`
	Ts      time.Time `json:"timestamp"`
}

func (e WatcherEvent) EventType() string         { return TypeWatcherEvent }
func (e WatcherEvent) EventTimestamp() time.Time { return e.Ts }

// DreamCycleCompleted is emitted when the dreaming agent completes a replay cycle.
type DreamCycleCompleted struct {
	MemoriesReplayed         int       `json:"memories_replayed"`
	AssociationsStrengthened int       `json:"associations_strengthened"`
	NewAssociationsCreated   int       `json:"new_associations_created"`
	CrossProjectLinks        int       `json:"cross_project_links"`
	PatternLinks             int       `json:"pattern_links"`
	InsightsGenerated        int       `json:"insights_generated"`
	NoisyMemoriesDemoted     int       `json:"noisy_memories_demoted"`
	DurationMs               int64     `json:"duration_ms"`
	Ts                       time.Time `json:"timestamp"`
}

func (e DreamCycleCompleted) EventType() string         { return TypeDreamCycleCompleted }
func (e DreamCycleCompleted) EventTimestamp() time.Time { return e.Ts }

// EpisodeClosed is emitted when an episode is synthesized and closed.
type EpisodeClosed struct {
	EpisodeID   string    `json:"episode_id"`
	Title       string    `json:"title"`
	EventCount  int       `json:"event_count"`
	DurationSec int       `json:"duration_sec"`
	Ts          time.Time `json:"timestamp"`
}

func (e EpisodeClosed) EventType() string         { return TypeEpisodeClosed }
func (e EpisodeClosed) EventTimestamp() time.Time { return e.Ts }

// PatternDiscovered is emitted when a new pattern is extracted from memory clusters.
type PatternDiscovered struct {
	PatternID     string    `json:"pattern_id"`
	Title         string    `json:"title"`
	PatternType   string    `json:"pattern_type"`
	Project       string    `json:"project,omitempty"`
	EvidenceCount int       `json:"evidence_count"`
	Ts            time.Time `json:"timestamp"`
}

func (e PatternDiscovered) EventType() string         { return TypePatternDiscovered }
func (e PatternDiscovered) EventTimestamp() time.Time { return e.Ts }

// AbstractionCreated is emitted when a new principle or axiom is synthesized.
const TypeAbstractionCreated = "abstraction_created"

type AbstractionCreated struct {
	AbstractionID string    `json:"abstraction_id"`
	Level         int       `json:"level"` // 2=principle, 3=axiom
	Title         string    `json:"title"`
	SourceCount   int       `json:"source_count"`
	Ts            time.Time `json:"timestamp"`
}

func (e AbstractionCreated) EventType() string         { return TypeAbstractionCreated }
func (e AbstractionCreated) EventTimestamp() time.Time { return e.Ts }

// AssocCandidate is a pending association for LLM reclassification.
type AssocCandidate struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Summary1 string `json:"summary1"`
	Summary2 string `json:"summary2"`
}

// AssociationsPendingClassification is emitted when associations default to "similar" and
// may benefit from LLM-based reclassification to more specific types.
type AssociationsPendingClassification struct {
	Candidates []AssocCandidate `json:"candidates"`
	Ts         time.Time        `json:"timestamp"`
}

const TypeAssociationsPendingClassification = "associations_pending_classification"

func (e AssociationsPendingClassification) EventType() string {
	return TypeAssociationsPendingClassification
}
func (e AssociationsPendingClassification) EventTimestamp() time.Time { return e.Ts }

// MemoryAmended is emitted when a memory's content is updated in place.
const TypeMemoryAmended = "memory_amended"

type MemoryAmended struct {
	MemoryID   string    `json:"memory_id"`
	OldSummary string    `json:"old_summary"`
	NewSummary string    `json:"new_summary"`
	Ts         time.Time `json:"timestamp"`
}

func (e MemoryAmended) EventType() string         { return TypeMemoryAmended }
func (e MemoryAmended) EventTimestamp() time.Time { return e.Ts }

// SessionEnded is emitted when an MCP session disconnects (stdin EOF).
type SessionEnded struct {
	SessionID string    `json:"session_id"`
	Project   string    `json:"project"`
	Ts        time.Time `json:"timestamp"`
}

const TypeSessionEnded = "session_ended"

func (e SessionEnded) EventType() string         { return TypeSessionEnded }
func (e SessionEnded) EventTimestamp() time.Time { return e.Ts }
