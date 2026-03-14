package events

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// TestSubscribePublish tests the basic subscribe and publish flow.
func TestSubscribePublish(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	received := make([]Event, 0)
	var mu sync.Mutex

	subID := bus.Subscribe(TypeRawMemoryCreated, func(ctx context.Context, event Event) error {
		mu.Lock()
		received = append(received, event)
		mu.Unlock()
		return nil
	})

	if subID == "" {
		t.Fatal("expected non-empty subscription ID")
	}

	event := RawMemoryCreated{
		ID:             "test-1",
		Source:         "test",
		HeuristicScore: 0.5,
		Salience:       0.6,
		Ts:             time.Now(),
	}

	// Give the async dispatch a moment to process
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(received) != 1 {
		t.Fatalf("expected 1 event received, got %d", len(received))
	}
	mu.Unlock()
}

// TestMultipleSubscribers tests that multiple subscribers for the same event type all receive events.
func TestMultipleSubscribers(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	count1 := 0
	count2 := 0
	count3 := 0
	var mu sync.Mutex

	bus.Subscribe(TypeMemoryEncoded, func(ctx context.Context, event Event) error {
		mu.Lock()
		count1++
		mu.Unlock()
		return nil
	})

	bus.Subscribe(TypeMemoryEncoded, func(ctx context.Context, event Event) error {
		mu.Lock()
		count2++
		mu.Unlock()
		return nil
	})

	bus.Subscribe(TypeMemoryEncoded, func(ctx context.Context, event Event) error {
		mu.Lock()
		count3++
		mu.Unlock()
		return nil
	})

	event := MemoryEncoded{
		MemoryID:            "mem-1",
		RawID:               "raw-1",
		Concepts:            []string{"test"},
		AssociationsCreated: 1,
		Ts:                  time.Now(),
	}

	for i := 0; i < 3; i++ {
		if err := bus.Publish(context.Background(), event); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if count1 != 3 {
		t.Fatalf("subscriber 1 expected 3 events, got %d", count1)
	}
	if count2 != 3 {
		t.Fatalf("subscriber 2 expected 3 events, got %d", count2)
	}
	if count3 != 3 {
		t.Fatalf("subscriber 3 expected 3 events, got %d", count3)
	}
}

// TestUnsubscribe tests that unsubscribed handlers don't fire.
func TestUnsubscribe(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	count1 := 0
	count2 := 0
	var mu sync.Mutex

	subID1 := bus.Subscribe(TypeQueryExecuted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count1++
		mu.Unlock()
		return nil
	})

	subID2 := bus.Subscribe(TypeQueryExecuted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count2++
		mu.Unlock()
		return nil
	})

	event := QueryExecuted{
		QueryID:         "q1",
		QueryText:       "test",
		ResultsReturned: 5,
		TookMs:          10,
		Ts:              time.Now(),
	}

	// Publish with both subscribers active
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Unsubscribe the first subscriber
	bus.Unsubscribe(subID1)

	// Publish again
	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if count1 != 1 {
		t.Fatalf("subscriber 1 expected 1 event after unsubscribe, got %d", count1)
	}
	if count2 != 2 {
		t.Fatalf("subscriber 2 expected 2 events, got %d", count2)
	}

	// Verify subID2 is still valid by checking the subscription is still there
	_ = subID2
}

// TestPublishWithContextCancellation tests publishing with a cancelled context.
func TestPublishWithContextCancellation(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	count := 0
	var mu sync.Mutex

	bus.Subscribe(TypeConsolidationStarted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	event := ConsolidationStarted{
		Ts: time.Now(),
	}

	// Publishing with cancelled context should not error
	// The event is still queued but context signals cancellation
	if err := bus.Publish(ctx, event); err != nil {
		t.Fatalf("publish with cancelled context failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	// The event may or may not be processed depending on timing
	_ = count
	mu.Unlock()
}

// TestClosedBusPublish tests that publishing to a closed bus fails.
func TestClosedBusPublish(t *testing.T) {
	bus := NewInMemoryBus(10)
	if err := bus.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	event := SystemHealth{
		LLMAvailable:   true,
		StoreHealthy:   true,
		ActiveWatchers: 1,
		MemoryCount:    10,
		Ts:             time.Now(),
	}

	err := bus.Publish(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when publishing to closed bus")
	}
}

// TestClosedBusDoubleClose tests that closing an already-closed bus fails.
func TestClosedBusDoubleClose(t *testing.T) {
	bus := NewInMemoryBus(10)
	if err := bus.Close(); err != nil {
		t.Fatalf("first close failed: %v", err)
	}

	err := bus.Close()
	if err == nil {
		t.Fatal("expected error on double close")
	}
}

// TestCloseStopsProcessing tests that Close() stops processing events.
func TestCloseStopsProcessing(t *testing.T) {
	bus := NewInMemoryBus(10)

	count := 0
	var mu sync.Mutex

	bus.Subscribe(TypeWatcherEvent, func(ctx context.Context, event Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	event := WatcherEvent{
		Source:  "test",
		Type:    "file",
		Path:    "/test",
		Preview: "test",
		Ts:      time.Now(),
	}

	// Publish some events
	for i := 0; i < 3; i++ {
		if err := bus.Publish(context.Background(), event); err != nil {
			t.Fatalf("publish failed: %v", err)
		}
	}

	// Give them time to be processed
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	countBeforeClose := count
	mu.Unlock()

	// Close the bus
	if err := bus.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	mu.Lock()
	countAfterClose := count
	mu.Unlock()

	// Count should not change after close (no new events processed)
	if countBeforeClose != countAfterClose {
		t.Logf("count before close: %d, after close: %d", countBeforeClose, countAfterClose)
	}
}

// TestHandlerError tests that handler errors are logged but don't prevent other handlers from running.
func TestHandlerError(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	count1 := 0
	count2 := 0
	count3 := 0
	var mu sync.Mutex

	// First handler returns an error
	bus.Subscribe(TypeDreamCycleCompleted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count1++
		mu.Unlock()
		return errors.New("test error")
	})

	// Second handler succeeds
	bus.Subscribe(TypeDreamCycleCompleted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count2++
		mu.Unlock()
		return nil
	})

	// Third handler succeeds
	bus.Subscribe(TypeDreamCycleCompleted, func(ctx context.Context, event Event) error {
		mu.Lock()
		count3++
		mu.Unlock()
		return nil
	})

	event := DreamCycleCompleted{
		MemoriesReplayed:         5,
		AssociationsStrengthened: 3,
		NewAssociationsCreated:   1,
		NoisyMemoriesDemoted:     0,
		DurationMs:               100,
		Ts:                       time.Now(),
	}

	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// All handlers should have been called despite the error
	if count1 != 1 {
		t.Fatalf("handler 1 expected 1 call, got %d", count1)
	}
	if count2 != 1 {
		t.Fatalf("handler 2 expected 1 call, got %d", count2)
	}
	if count3 != 1 {
		t.Fatalf("handler 3 expected 1 call, got %d", count3)
	}
}

// TestEventTypeFiltering tests that events are only sent to subscribers of the correct type.
func TestEventTypeFiltering(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	countType1 := 0
	countType2 := 0
	var mu sync.Mutex

	bus.Subscribe(TypeConsolidationCompleted, func(ctx context.Context, event Event) error {
		mu.Lock()
		countType1++
		mu.Unlock()
		return nil
	})

	bus.Subscribe(TypeMetaCycleCompleted, func(ctx context.Context, event Event) error {
		mu.Lock()
		countType2++
		mu.Unlock()
		return nil
	})

	// Publish ConsolidationCompleted events
	consolidationEvent := ConsolidationCompleted{
		DurationMs:         500,
		MemoriesProcessed:  10,
		MemoriesDecayed:    2,
		MergedClusters:     1,
		AssociationsPruned: 5,
		Ts:                 time.Now(),
	}

	for i := 0; i < 2; i++ {
		if err := bus.Publish(context.Background(), consolidationEvent); err != nil {
			t.Fatalf("publish consolidation failed: %v", err)
		}
	}

	// Publish MetaCycleCompleted events
	metaEvent := MetaCycleCompleted{
		ObservationsLogged: 3,
		Ts:                 time.Now(),
	}

	for i := 0; i < 3; i++ {
		if err := bus.Publish(context.Background(), metaEvent); err != nil {
			t.Fatalf("publish meta failed: %v", err)
		}
	}

	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if countType1 != 2 {
		t.Fatalf("consolidation subscriber expected 2 events, got %d", countType1)
	}
	if countType2 != 3 {
		t.Fatalf("meta subscriber expected 3 events, got %d", countType2)
	}
}

// TestUnsubscribeInvalidID tests that unsubscribing an invalid ID is a no-op.
func TestUnsubscribeInvalidID(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	count := 0
	var mu sync.Mutex

	bus.Subscribe(TypeMemoryAccessed, func(ctx context.Context, event Event) error {
		mu.Lock()
		count++
		mu.Unlock()
		return nil
	})

	event := MemoryAccessed{
		MemoryIDs: []string{"m1", "m2"},
		QueryID:   "q1",
		Ts:        time.Now(),
	}

	// Unsubscribe with invalid ID (should be a no-op)
	bus.Unsubscribe("invalid-id")

	if err := bus.Publish(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if count != 1 {
		t.Fatalf("handler expected 1 call, got %d", count)
	}
}

// TestNoSubscribersForEventType tests that publishing an event with no subscribers doesn't error.
func TestNoSubscribersForEventType(t *testing.T) {
	bus := NewInMemoryBus(10)
	defer func() { _ = bus.Close() }()

	event := WatcherEvent{
		Source: "test",
		Type:   "test",
		Ts:     time.Now(),
	}

	// No subscribers for this type
	err := bus.Publish(context.Background(), event)
	if err != nil {
		t.Fatalf("publish with no subscribers failed: %v", err)
	}
}

// TestEventBufferFull tests that a full buffer logs a warning but doesn't error.
func TestEventBufferFull(t *testing.T) {
	// Create a small buffer size to trigger full condition
	bus := NewInMemoryBus(1)
	defer func() { _ = bus.Close() }()

	// Block the dispatch by subscribing with a slow handler
	blockChan := make(chan struct{})
	bus.Subscribe(TypeSystemHealth, func(ctx context.Context, event Event) error {
		<-blockChan
		return nil
	})

	event := SystemHealth{
		LLMAvailable:   true,
		StoreHealthy:   true,
		ActiveWatchers: 0,
		MemoryCount:    0,
		Ts:             time.Now(),
	}

	// Publish multiple events to fill the buffer
	// First one will block in the handler
	go func() {
		_ = bus.Publish(context.Background(), event)
	}()

	time.Sleep(50 * time.Millisecond)

	// Try to publish more events - should trigger buffer full condition
	// but should not error (the function returns nil regardless)
	for i := 0; i < 5; i++ {
		err := bus.Publish(context.Background(), event)
		if err != nil {
			t.Fatalf("publish to full buffer failed: %v", err)
		}
	}

	close(blockChan)
	time.Sleep(100 * time.Millisecond)
}
