package reactor

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/store"
)

// ---------------------------------------------------------------------------
// Mock Store — implements store.Store with only the methods reactor needs.
// All other methods are stubs that return zero values.
// ---------------------------------------------------------------------------

type mockStore struct {
	observations []store.MetaObservation
	writtenObs   []store.MetaObservation
	statistics   store.StoreStatistics
}

func (m *mockStore) ListMetaObservations(_ context.Context, obsType string, limit int) ([]store.MetaObservation, error) {
	var result []store.MetaObservation
	for _, obs := range m.observations {
		if obsType != "" && obs.ObservationType != obsType {
			continue
		}
		result = append(result, obs)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockStore) WriteMetaObservation(_ context.Context, obs store.MetaObservation) error {
	m.writtenObs = append(m.writtenObs, obs)
	return nil
}

func (m *mockStore) GetStatistics(_ context.Context) (store.StoreStatistics, error) {
	return m.statistics, nil
}

// --- All other store.Store methods (stubs) ---

func (m *mockStore) WriteRaw(context.Context, store.RawMemory) error { return nil }
func (m *mockStore) GetRaw(context.Context, string) (store.RawMemory, error) {
	return store.RawMemory{}, nil
}
func (m *mockStore) ListRawUnprocessed(context.Context, int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) ListRawMemoriesAfter(context.Context, time.Time, int) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) MarkRawProcessed(context.Context, string) error  { return nil }
func (m *mockStore) WriteMemory(context.Context, store.Memory) error { return nil }
func (m *mockStore) GetMemory(context.Context, string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) GetMemoryByRawID(context.Context, string) (store.Memory, error) {
	return store.Memory{}, nil
}
func (m *mockStore) UpdateMemory(context.Context, store.Memory) error      { return nil }
func (m *mockStore) UpdateSalience(context.Context, string, float32) error { return nil }
func (m *mockStore) UpdateState(context.Context, string, string) error     { return nil }
func (m *mockStore) IncrementAccess(context.Context, string) error         { return nil }
func (m *mockStore) ListMemories(context.Context, string, int, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) CountMemories(context.Context) (int, error) { return 0, nil }
func (m *mockStore) SearchByFullText(context.Context, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) SearchByEmbedding(context.Context, []float32, int) ([]store.RetrievalResult, error) {
	return nil, nil
}
func (m *mockStore) SearchByConcepts(context.Context, []string, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) CreateAssociation(context.Context, store.Association) error { return nil }
func (m *mockStore) GetAssociations(context.Context, string) ([]store.Association, error) {
	return nil, nil
}
func (m *mockStore) UpdateAssociationStrength(context.Context, string, string, float32) error {
	return nil
}
func (m *mockStore) UpdateAssociationType(context.Context, string, string, string) error {
	return nil
}
func (m *mockStore) WriteRetrievalFeedback(context.Context, store.RetrievalFeedback) error {
	return nil
}
func (m *mockStore) GetRetrievalFeedback(context.Context, string) (store.RetrievalFeedback, error) {
	return store.RetrievalFeedback{}, nil
}
func (m *mockStore) ActivateAssociation(context.Context, string, string) error { return nil }
func (m *mockStore) PruneWeakAssociations(context.Context, float32) (int, error) {
	return 0, nil
}
func (m *mockStore) BatchUpdateSalience(context.Context, map[string]float32) error { return nil }
func (m *mockStore) BatchMergeMemories(context.Context, []string, store.Memory) error {
	return nil
}
func (m *mockStore) DeleteOldArchived(context.Context, time.Time) (int, error) { return 0, nil }
func (m *mockStore) WriteConsolidation(context.Context, store.ConsolidationRecord) error {
	return nil
}
func (m *mockStore) GetLastConsolidation(context.Context) (store.ConsolidationRecord, error) {
	return store.ConsolidationRecord{}, nil
}
func (m *mockStore) ListAllAssociations(context.Context) ([]store.Association, error) {
	return nil, nil
}
func (m *mockStore) ListAllRawMemories(context.Context) ([]store.RawMemory, error) {
	return nil, nil
}
func (m *mockStore) GetDeadMemories(context.Context, time.Time) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) GetSourceDistribution(context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockStore) CreateEpisode(context.Context, store.Episode) error { return nil }
func (m *mockStore) GetEpisode(context.Context, string) (store.Episode, error) {
	return store.Episode{}, nil
}
func (m *mockStore) UpdateEpisode(context.Context, store.Episode) error { return nil }
func (m *mockStore) ListEpisodes(context.Context, string, int, int) ([]store.Episode, error) {
	return nil, nil
}
func (m *mockStore) GetOpenEpisode(context.Context) (store.Episode, error) {
	return store.Episode{}, nil
}
func (m *mockStore) CloseEpisode(context.Context, string) error { return nil }
func (m *mockStore) WriteMemoryResolution(context.Context, store.MemoryResolution) error {
	return nil
}
func (m *mockStore) GetMemoryResolution(context.Context, string) (store.MemoryResolution, error) {
	return store.MemoryResolution{}, nil
}
func (m *mockStore) WriteConceptSet(context.Context, store.ConceptSet) error { return nil }
func (m *mockStore) GetConceptSet(context.Context, string) (store.ConceptSet, error) {
	return store.ConceptSet{}, nil
}
func (m *mockStore) SearchByEntity(context.Context, string, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) WriteMemoryAttributes(context.Context, store.MemoryAttributes) error {
	return nil
}
func (m *mockStore) GetMemoryAttributes(context.Context, string) (store.MemoryAttributes, error) {
	return store.MemoryAttributes{}, nil
}
func (m *mockStore) WritePattern(context.Context, store.Pattern) error { return nil }
func (m *mockStore) GetPattern(context.Context, string) (store.Pattern, error) {
	return store.Pattern{}, nil
}
func (m *mockStore) UpdatePattern(context.Context, store.Pattern) error { return nil }
func (m *mockStore) ListPatterns(context.Context, string, int) ([]store.Pattern, error) {
	return nil, nil
}
func (m *mockStore) SearchPatternsByEmbedding(context.Context, []float32, int) ([]store.Pattern, error) {
	return nil, nil
}
func (m *mockStore) WriteAbstraction(context.Context, store.Abstraction) error { return nil }
func (m *mockStore) GetAbstraction(context.Context, string) (store.Abstraction, error) {
	return store.Abstraction{}, nil
}
func (m *mockStore) UpdateAbstraction(context.Context, store.Abstraction) error { return nil }
func (m *mockStore) ListAbstractions(context.Context, int, int) ([]store.Abstraction, error) {
	return nil, nil
}
func (m *mockStore) SearchAbstractionsByEmbedding(context.Context, []float32, int) ([]store.Abstraction, error) {
	return nil, nil
}
func (m *mockStore) SearchByProject(context.Context, string, string, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) ListMemoriesByTimeRange(context.Context, time.Time, time.Time, int) ([]store.Memory, error) {
	return nil, nil
}
func (m *mockStore) GetProjectSummary(context.Context, string) (map[string]interface{}, error) {
	return nil, nil
}
func (m *mockStore) ListProjects(context.Context) ([]string, error) { return nil, nil }
func (m *mockStore) RawMemoryExistsByPath(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (m *mockStore) CountRawUnprocessedByPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}
func (m *mockStore) BulkMarkRawProcessedByPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}
func (m *mockStore) ArchiveMemoriesByRawPathPatterns(context.Context, []string) (int, error) {
	return 0, nil
}
func (m *mockStore) BatchWriteRaw(context.Context, []store.RawMemory) error { return nil }
func (m *mockStore) Close() error                                           { return nil }

// ---------------------------------------------------------------------------
// Helper: synchronous bus for deterministic tests
// ---------------------------------------------------------------------------

type syncBus struct {
	handlers map[string][]events.Handler
}

func newSyncBus() *syncBus {
	return &syncBus{handlers: make(map[string][]events.Handler)}
}

func (b *syncBus) Publish(ctx context.Context, event events.Event) error {
	for _, h := range b.handlers[event.EventType()] {
		if err := h(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (b *syncBus) Subscribe(eventType string, handler events.Handler) string {
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return eventType // subscription ID not needed for tests
}

func (b *syncBus) Unsubscribe(string) {}
func (b *syncBus) Close() error       { return nil }

// ---------------------------------------------------------------------------
// Test logger
// ---------------------------------------------------------------------------

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// ===========================================================================
// Condition Tests
// ===========================================================================

func TestCooldownCondition_FirstCall(t *testing.T) {
	state := NewReactorState(&mockStore{}, newSyncBus())
	cond := &CooldownCondition{ChainID: "test", Duration: 5 * time.Minute}

	met, err := cond.Evaluate(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if !met {
		t.Error("expected cooldown to pass on first call (no prior execution)")
	}
}

func TestCooldownCondition_WithinWindow(t *testing.T) {
	state := NewReactorState(&mockStore{}, newSyncBus())
	state.LastExecution["test"] = time.Now()

	cond := &CooldownCondition{ChainID: "test", Duration: 5 * time.Minute}

	met, err := cond.Evaluate(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if met {
		t.Error("expected cooldown to block within window")
	}
}

func TestCooldownCondition_AfterWindow(t *testing.T) {
	state := NewReactorState(&mockStore{}, newSyncBus())
	state.LastExecution["test"] = time.Now().Add(-10 * time.Minute)

	cond := &CooldownCondition{ChainID: "test", Duration: 5 * time.Minute}

	met, err := cond.Evaluate(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if !met {
		t.Error("expected cooldown to pass after window")
	}
}

func TestObservationSeverityCondition_Warning(t *testing.T) {
	ms := &mockStore{
		observations: []store.MetaObservation{
			{ObservationType: "recall_effectiveness", Severity: "warning"},
		},
	}
	state := NewReactorState(ms, newSyncBus())
	cond := &ObservationSeverityCondition{ObservationType: "recall_effectiveness", MinSeverity: "warning"}

	met, err := cond.Evaluate(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if !met {
		t.Error("expected warning to meet warning threshold")
	}
}

func TestObservationSeverityCondition_InfoBelowWarning(t *testing.T) {
	ms := &mockStore{
		observations: []store.MetaObservation{
			{ObservationType: "recall_effectiveness", Severity: "info"},
		},
	}
	state := NewReactorState(ms, newSyncBus())
	cond := &ObservationSeverityCondition{ObservationType: "recall_effectiveness", MinSeverity: "warning"}

	met, err := cond.Evaluate(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if met {
		t.Error("expected info to NOT meet warning threshold")
	}
}

func TestDBSizeCondition_Exceeds(t *testing.T) {
	ms := &mockStore{
		statistics: store.StoreStatistics{
			ActiveMemories:   8000,
			FadingMemories:   2000,
			ArchivedMemories: 1000,
		},
	}
	state := NewReactorState(ms, newSyncBus())
	// 11000 * 10 / 1024 ≈ 107 MB
	cond := &DBSizeCondition{MaxSizeMB: 100}

	met, err := cond.Evaluate(context.Background(), events.SystemHealth{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if !met {
		t.Error("expected DB size condition to pass when size exceeds threshold")
	}
}

func TestDBSizeCondition_Below(t *testing.T) {
	ms := &mockStore{
		statistics: store.StoreStatistics{
			ActiveMemories:   100,
			FadingMemories:   50,
			ArchivedMemories: 20,
		},
	}
	state := NewReactorState(ms, newSyncBus())
	// 170 * 10 / 1024 ≈ 1 MB
	cond := &DBSizeCondition{MaxSizeMB: 100}

	met, err := cond.Evaluate(context.Background(), events.SystemHealth{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if met {
		t.Error("expected DB size condition to fail when size is below threshold")
	}
}

// ===========================================================================
// Action Tests
// ===========================================================================

func TestPublishEventAction(t *testing.T) {
	bus := newSyncBus()
	state := NewReactorState(&mockStore{}, bus)

	var received bool
	bus.Subscribe(events.TypeConsolidationStarted, func(_ context.Context, _ events.Event) error {
		received = true
		return nil
	})

	action := &PublishEventAction{
		EventFactory: func() events.Event {
			return events.ConsolidationStarted{Ts: time.Now()}
		},
		Log: testLogger(),
	}

	err := action.Execute(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if !received {
		t.Error("expected event to be published and received")
	}
}

func TestSendToChannelAction_Success(t *testing.T) {
	ch := make(chan struct{}, 1)
	action := &SendToChannelAction{
		ChannelName: "test",
		Channel:     ch,
		Log:         testLogger(),
	}

	err := action.Execute(context.Background(), events.ConsolidationStarted{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case <-ch:
		// success
	default:
		t.Error("expected signal on channel")
	}
}

func TestSendToChannelAction_Full(t *testing.T) {
	ch := make(chan struct{}, 1)
	ch <- struct{}{} // fill it

	action := &SendToChannelAction{
		ChannelName: "test",
		Channel:     ch,
		Log:         testLogger(),
	}

	// Should not block or error
	err := action.Execute(context.Background(), events.ConsolidationStarted{}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogMetaObservationAction(t *testing.T) {
	ms := &mockStore{}
	state := NewReactorState(ms, newSyncBus())

	action := &LogMetaObservationAction{
		ActionName:  "requested_consolidation",
		TriggerName: "high_dead_ratio",
		Log:         testLogger(),
	}

	err := action.Execute(context.Background(), events.MetaCycleCompleted{}, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(ms.writtenObs) != 1 {
		t.Fatalf("expected 1 observation written, got %d", len(ms.writtenObs))
	}
	if ms.writtenObs[0].ObservationType != "autonomous_action" {
		t.Errorf("expected observation type 'autonomous_action', got %q", ms.writtenObs[0].ObservationType)
	}
}

func TestIncrementCounterAction(t *testing.T) {
	var count int
	action := &IncrementCounterAction{
		CounterName: "test",
		Increment:   func() { count++ },
	}

	_ = action.Execute(context.Background(), events.SystemHealth{}, nil)
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

// ===========================================================================
// Engine Tests
// ===========================================================================

func TestEngine_ChainFires(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()

	engine := NewEngine(ms, bus, log)

	fired := false
	engine.RegisterChain(&Chain{
		ID:      "test_chain",
		Name:    "Test Chain",
		Trigger: EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{
			&AlwaysTrueCondition{},
		},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "test",
				Increment:   func() { fired = true },
			},
		},
		Priority: 10,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}

	// Publish event — sync bus means it's handled immediately
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if !fired {
		t.Error("expected chain to fire")
	}
}

func TestEngine_CooldownPreventsRefire(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()

	engine := NewEngine(ms, bus, log)

	fireCount := 0
	engine.RegisterChain(&Chain{
		ID:      "cooldown_chain",
		Name:    "Cooldown Chain",
		Trigger: EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{
			&CooldownCondition{ChainID: "cooldown_chain", Duration: 5 * time.Minute},
		},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "test",
				Increment:   func() { fireCount++ },
			},
		},
		Priority: 10,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}

	// First publish — should fire
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Second publish — should be blocked by cooldown
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if fireCount != 1 {
		t.Errorf("expected chain to fire exactly once (cooldown), got %d", fireCount)
	}
}

func TestEngine_PriorityOrdering(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()

	engine := NewEngine(ms, bus, log)

	var order []string
	engine.RegisterChain(&Chain{
		ID:         "low_priority",
		Name:       "Low Priority",
		Trigger:    EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{&AlwaysTrueCondition{}},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "low",
				Increment:   func() { order = append(order, "low") },
			},
		},
		Priority: 1,
		Enabled:  true,
	})
	engine.RegisterChain(&Chain{
		ID:         "high_priority",
		Name:       "High Priority",
		Trigger:    EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{&AlwaysTrueCondition{}},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "high",
				Increment:   func() { order = append(order, "high") },
			},
		},
		Priority: 100,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if len(order) != 2 {
		t.Fatalf("expected 2 chains to fire, got %d", len(order))
	}
	if order[0] != "high" || order[1] != "low" {
		t.Errorf("expected [high, low], got %v", order)
	}
}

func TestEngine_DisabledChain(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()

	engine := NewEngine(ms, bus, log)

	fired := false
	engine.RegisterChain(&Chain{
		ID:         "disabled_chain",
		Name:       "Disabled Chain",
		Trigger:    EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{&AlwaysTrueCondition{}},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "test",
				Increment:   func() { fired = true },
			},
		},
		Priority: 10,
		Enabled:  false,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if fired {
		t.Error("expected disabled chain to NOT fire")
	}
}

func TestEngine_ConditionBlocking(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{
		observations: []store.MetaObservation{
			{ObservationType: "recall_effectiveness", Severity: "info"}, // below "warning"
		},
	}
	log := testLogger()

	engine := NewEngine(ms, bus, log)

	fired := false
	engine.RegisterChain(&Chain{
		ID:      "blocked_chain",
		Name:    "Blocked Chain",
		Trigger: EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{
			&ObservationSeverityCondition{
				ObservationType: "recall_effectiveness",
				MinSeverity:     "warning",
			},
		},
		Actions: []Action{
			&IncrementCounterAction{
				CounterName: "test",
				Increment:   func() { fired = true },
			},
		},
		Priority: 10,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{Ts: time.Now()}); err != nil {
		t.Fatal(err)
	}

	if fired {
		t.Error("expected chain to NOT fire when observation severity is below threshold")
	}
}

func TestEngine_FullConsolidationChain(t *testing.T) {
	// Simulate the real flow: MetaCycleCompleted → publish ConsolidationStarted → send to triggerCh
	bus := newSyncBus()
	ms := &mockStore{
		observations: []store.MetaObservation{
			{ObservationType: "recall_effectiveness", Severity: "critical"},
		},
	}
	log := testLogger()
	triggerCh := make(chan struct{}, 1)

	engine := NewEngine(ms, bus, log)

	// Chain 1: MetaCycleCompleted → publish ConsolidationStarted
	engine.RegisterChain(&Chain{
		ID:      "meta_consolidation",
		Name:    "Meta → Consolidation",
		Trigger: EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		Conditions: []Condition{
			&ObservationSeverityCondition{
				ObservationType: "recall_effectiveness",
				MinSeverity:     "warning",
			},
		},
		Actions: []Action{
			&PublishEventAction{
				EventFactory: func() events.Event {
					return events.ConsolidationStarted{Ts: time.Now()}
				},
				Log: log,
			},
		},
		Priority: 10,
		Enabled:  true,
	})

	// Chain 3: ConsolidationStarted → send to triggerCh
	engine.RegisterChain(&Chain{
		ID:      "consolidation_trigger",
		Name:    "ConsolidationStarted → Agent",
		Trigger: EventTypeMatcher{EventType: events.TypeConsolidationStarted},
		Conditions: []Condition{
			&CooldownCondition{ChainID: "consolidation_trigger", Duration: 5 * time.Minute},
		},
		Actions: []Action{
			&SendToChannelAction{
				ChannelName: "consolidation",
				Channel:     triggerCh,
				Log:         log,
			},
		},
		Priority: 100,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}

	// Trigger the chain
	if err := bus.Publish(context.Background(), events.MetaCycleCompleted{
		ObservationsLogged: 3,
		Ts:                 time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	// The sync bus should have cascaded: MetaCycleCompleted → ConsolidationStarted → triggerCh
	select {
	case <-triggerCh:
		// success — full chain cascade worked
	default:
		t.Error("expected consolidation trigger channel to receive signal from chain cascade")
	}
}

func TestNewChainRegistry(t *testing.T) {
	consolTrigger := make(chan struct{}, 1)
	abstrTrigger := make(chan struct{}, 1)
	metaTrigger := make(chan struct{}, 1)
	dreamTrigger := make(chan struct{}, 1)

	chains := NewChainRegistry(ChainDeps{
		ConsolidationTrigger: consolTrigger,
		AbstractionTrigger:   abstrTrigger,
		MetacognitionTrigger: metaTrigger,
		DreamingTrigger:      dreamTrigger,
		IncrementAutonomous:  func() {},
		MaxDBSizeMB:          100,
		Logger:               testLogger(),
	})

	if len(chains) != 6 {
		t.Errorf("expected 6 chains, got %d", len(chains))
	}

	// Verify chain IDs
	ids := make(map[string]bool)
	for _, c := range chains {
		ids[c.ID] = true
	}

	expected := []string{
		"meta_consolidation_on_dead_ratio",
		"orch_consolidation_on_db_size",
		"consolidation_on_request",
		"abstraction_on_pattern",
		"meta_on_consolidation_completed",
		"dreaming_on_episode_closed",
	}
	for _, id := range expected {
		if !ids[id] {
			t.Errorf("expected chain %q to be registered", id)
		}
	}
}

func TestMetaOnConsolidationCompletedChain(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()
	metaTrigger := make(chan struct{}, 1)

	engine := NewEngine(ms, bus, log)

	engine.RegisterChain(&Chain{
		ID:          "meta_on_consolidation_completed",
		Name:        "Metacognition: Execute After Consolidation",
		Trigger:     EventTypeMatcher{EventType: events.TypeConsolidationCompleted},
		TriggerType: events.TypeConsolidationCompleted,
		Conditions: []Condition{
			&CooldownCondition{
				ChainID:  "meta_on_consolidation_completed",
				Duration: 30 * time.Minute,
			},
		},
		Actions: []Action{
			&SendToChannelAction{
				ChannelName: "metacognition_trigger",
				Channel:     metaTrigger,
				Log:         log,
			},
		},
		Cooldown: 30 * time.Minute,
		Priority: 40,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}

	if err := bus.Publish(context.Background(), events.ConsolidationCompleted{
		DurationMs:        1000,
		MemoriesProcessed: 10,
		Ts:                time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-metaTrigger:
		// success
	default:
		t.Error("expected metacognition trigger to fire after ConsolidationCompleted")
	}
}

func TestDreamingOnEpisodeClosedChain(t *testing.T) {
	bus := newSyncBus()
	ms := &mockStore{}
	log := testLogger()
	dreamTrigger := make(chan struct{}, 1)

	engine := NewEngine(ms, bus, log)

	engine.RegisterChain(&Chain{
		ID:          "dreaming_on_episode_closed",
		Name:        "Dreaming: Execute After Episode Closes",
		Trigger:     EventTypeMatcher{EventType: events.TypeEpisodeClosed},
		TriggerType: events.TypeEpisodeClosed,
		Conditions: []Condition{
			&CooldownCondition{
				ChainID:  "dreaming_on_episode_closed",
				Duration: 10 * time.Minute,
			},
		},
		Actions: []Action{
			&SendToChannelAction{
				ChannelName: "dreaming_trigger",
				Channel:     dreamTrigger,
				Log:         log,
			},
		},
		Cooldown: 10 * time.Minute,
		Priority: 30,
		Enabled:  true,
	})

	if err := engine.Start(context.Background(), bus); err != nil {
		t.Fatal(err)
	}

	if err := bus.Publish(context.Background(), events.EpisodeClosed{
		EpisodeID:  "ep-123",
		Title:      "Test Episode",
		EventCount: 5,
		Ts:         time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-dreamTrigger:
		// success
	default:
		t.Error("expected dreaming trigger to fire after EpisodeClosed")
	}
}
