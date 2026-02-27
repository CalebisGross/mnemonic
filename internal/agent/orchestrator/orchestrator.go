package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
)

// OrchestratorConfig configures the autonomous orchestrator.
type OrchestratorConfig struct {
	AdaptiveIntervals  bool
	MaxDBSizeMB        int
	SelfTestInterval   time.Duration
	AutoRecovery       bool
	HealthReportPath   string // e.g. "~/.mnemonic/health.json"
	MonitorInterval    time.Duration
}

// HealthReport is the machine-readable health status written periodically.
type HealthReport struct {
	Timestamp          time.Time              `json:"timestamp"`
	Uptime             string                 `json:"uptime"`
	LLMAvailable       bool                   `json:"llm_available"`
	StoreHealthy       bool                   `json:"store_healthy"`
	MemoryCount        int                    `json:"memory_count"`
	PatternCount       int                    `json:"pattern_count"`
	AbstractionCount   int                    `json:"abstraction_count"`
	AgentStatus        map[string]string      `json:"agent_status"`
	LastConsolidation  string                 `json:"last_consolidation"`
	LastDreamCycle     string                 `json:"last_dream_cycle"`
	AutonomousActions  int                    `json:"autonomous_actions_total"`
	Warnings           []string               `json:"warnings,omitempty"`
}

// Orchestrator is the central autonomous scheduler and health monitor.
type Orchestrator struct {
	store       store.Store
	llmProvider llm.Provider
	config      OrchestratorConfig
	log         *slog.Logger
	bus         events.Bus
	startTime   time.Time

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopOnce sync.Once

	mu              sync.Mutex
	llmHealthy      bool
	lastLLMCheck    time.Time
	autonomousCount int
	warnings        []string
}

func NewOrchestrator(s store.Store, llmProv llm.Provider, cfg OrchestratorConfig, log *slog.Logger) *Orchestrator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Orchestrator{
		store:       s,
		llmProvider: llmProv,
		config:      cfg,
		log:         log,
		startTime:   time.Now(),
		ctx:         ctx,
		cancel:      cancel,
		llmHealthy:  true,
	}
}

func (o *Orchestrator) Name() string {
	return "orchestrator"
}

func (o *Orchestrator) Start(ctx context.Context, bus events.Bus) error {
	o.bus = bus

	// Run initial health checks
	o.checkLLMHealth(ctx)
	o.checkStoreHealth(ctx)

	// Start monitoring loop
	o.wg.Add(1)
	go o.monitorLoop()

	// Start self-test loop
	if o.config.SelfTestInterval > 0 {
		o.wg.Add(1)
		go o.selfTestLoop()
	}

	// Start health report writer
	if o.config.HealthReportPath != "" {
		o.wg.Add(1)
		go o.healthReportLoop()
	}

	o.log.Info("orchestrator started",
		"adaptive_intervals", o.config.AdaptiveIntervals,
		"auto_recovery", o.config.AutoRecovery,
		"monitor_interval", o.config.MonitorInterval)

	return nil
}

func (o *Orchestrator) Stop() error {
	o.stopOnce.Do(func() {
		o.cancel()
	})
	o.wg.Wait()
	return nil
}

func (o *Orchestrator) Health(ctx context.Context) error {
	_, err := o.store.CountMemories(ctx)
	return err
}

// monitorLoop runs periodic health checks and resource monitoring.
func (o *Orchestrator) monitorLoop() {
	defer o.wg.Done()

	interval := o.config.MonitorInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.runMonitorCycle(o.ctx)
		}
	}
}

func (o *Orchestrator) runMonitorCycle(ctx context.Context) {
	// Check LLM health
	o.checkLLMHealth(ctx)

	// Check store health
	o.checkStoreHealth(ctx)

	// Check DB size if configured
	if o.config.MaxDBSizeMB > 0 {
		o.checkDBSize(ctx)
	}

	// Publish system health event
	if o.bus != nil {
		memCount, _ := o.store.CountMemories(ctx)
		o.mu.Lock()
		healthy := o.llmHealthy
		o.mu.Unlock()

		_ = o.bus.Publish(ctx, events.SystemHealth{
			LLMAvailable: healthy,
			StoreHealthy: true,
			MemoryCount:  memCount,
			Ts:           time.Now(),
		})
	}
}

// checkLLMHealth verifies the LLM backend is reachable.
func (o *Orchestrator) checkLLMHealth(ctx context.Context) {
	if o.llmProvider == nil {
		return
	}

	err := o.llmProvider.Health(ctx)
	o.mu.Lock()
	defer o.mu.Unlock()

	wasHealthy := o.llmHealthy
	o.llmHealthy = err == nil
	o.lastLLMCheck = time.Now()

	if !o.llmHealthy && wasHealthy {
		o.log.Warn("LLM backend is unreachable", "error", err)
		o.addWarning("LLM backend unreachable")
	} else if o.llmHealthy && !wasHealthy {
		o.log.Info("LLM backend recovered")
		o.removeWarning("LLM backend unreachable")
	}
}

// checkStoreHealth verifies the SQLite store is healthy.
func (o *Orchestrator) checkStoreHealth(ctx context.Context) {
	_, err := o.store.CountMemories(ctx)
	if err != nil {
		o.log.Error("store health check failed", "error", err)
		o.mu.Lock()
		o.addWarning("Store health check failed")
		o.mu.Unlock()
	}
}

// checkDBSize checks if the database exceeds the configured max size.
// Consolidation triggering and cooldowns are now handled by the reactor engine's
// "orch_consolidation_on_db_size" chain, which fires on SystemHealth events.
func (o *Orchestrator) checkDBSize(ctx context.Context) {
	stats, err := o.store.GetStatistics(ctx)
	if err != nil {
		return
	}

	totalMemories := stats.ActiveMemories + stats.FadingMemories + stats.ArchivedMemories
	estimatedMB := totalMemories * 10 / 1024

	if estimatedMB > o.config.MaxDBSizeMB {
		o.log.Warn("database size exceeds threshold (reactor will handle consolidation)",
			"estimated_mb", estimatedMB,
			"max_mb", o.config.MaxDBSizeMB,
			"total_memories", totalMemories)

		o.mu.Lock()
		o.addWarning(fmt.Sprintf("DB size ~%dMB exceeds %dMB limit", estimatedMB, o.config.MaxDBSizeMB))
		o.mu.Unlock()
	}
}

// IncrementAutonomousCount increments the autonomous action counter.
// Used by the reactor engine to track autonomous consolidation triggers.
func (o *Orchestrator) IncrementAutonomousCount() {
	o.mu.Lock()
	o.autonomousCount++
	o.mu.Unlock()
}

// selfTestLoop periodically runs retrieval self-tests.
func (o *Orchestrator) selfTestLoop() {
	defer o.wg.Done()

	// Wait longer before first self-test
	timer := time.NewTimer(10 * time.Minute)
	defer timer.Stop()

	select {
	case <-o.ctx.Done():
		return
	case <-timer.C:
	}

	ticker := time.NewTicker(o.config.SelfTestInterval)
	defer ticker.Stop()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.runSelfTest(o.ctx)
		}
	}
}

// runSelfTest generates test queries from known patterns and verifies recall quality.
func (o *Orchestrator) runSelfTest(ctx context.Context) {
	o.mu.Lock()
	if !o.llmHealthy {
		o.mu.Unlock()
		return
	}
	o.mu.Unlock()

	// Load patterns to use as test queries
	patterns, err := o.store.ListPatterns(ctx, "", 5)
	if err != nil || len(patterns) == 0 {
		return
	}

	passCount := 0
	testCount := 0

	for _, pattern := range patterns {
		if len(pattern.EvidenceIDs) == 0 {
			continue
		}

		testCount++

		// Use pattern title as query — search by embedding
		if len(pattern.Embedding) == 0 {
			continue
		}

		results, err := o.store.SearchByEmbedding(ctx, pattern.Embedding, 10)
		if err != nil {
			continue
		}

		// Check if any evidence memories appear in results
		found := false
		for _, r := range results {
			for _, eid := range pattern.EvidenceIDs {
				if r.Memory.ID == eid {
					found = true
					break
				}
			}
			if found {
				break
			}
		}

		if found {
			passCount++
		}
	}

	if testCount > 0 {
		passRate := float64(passCount) / float64(testCount)
		o.log.Info("self-test completed", "tests", testCount, "passed", passCount, "rate", passRate)

		if passRate < 0.5 {
			o.log.Warn("self-test pass rate is low, retrieval quality may be degraded")
			o.mu.Lock()
			o.addWarning(fmt.Sprintf("Self-test pass rate: %.0f%%", passRate*100))
			o.autonomousCount++
			o.mu.Unlock()
		}
	}
}

// healthReportLoop writes periodic health reports to disk.
func (o *Orchestrator) healthReportLoop() {
	defer o.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	// Write initial report
	o.writeHealthReport()

	for {
		select {
		case <-o.ctx.Done():
			return
		case <-ticker.C:
			o.writeHealthReport()
		}
	}
}

func (o *Orchestrator) writeHealthReport() {
	ctx := context.Background()

	memCount, _ := o.store.CountMemories(ctx)
	stats, _ := o.store.GetStatistics(ctx)
	patterns, _ := o.store.ListPatterns(ctx, "", 100)
	level2, _ := o.store.ListAbstractions(ctx, 2, 100)
	level3, _ := o.store.ListAbstractions(ctx, 3, 100)

	lastConsolidation := "never"
	record, err := o.store.GetLastConsolidation(ctx)
	if err == nil && record.ID != "" {
		lastConsolidation = record.EndTime.Format(time.RFC3339)
	}

	o.mu.Lock()
	report := HealthReport{
		Timestamp:         time.Now(),
		Uptime:            time.Since(o.startTime).Round(time.Second).String(),
		LLMAvailable:      o.llmHealthy,
		StoreHealthy:      true,
		MemoryCount:       memCount,
		PatternCount:      len(patterns),
		AbstractionCount:  len(level2) + len(level3),
		LastConsolidation: lastConsolidation,
		AutonomousActions: o.autonomousCount,
		Warnings:          append([]string{}, o.warnings...),
		AgentStatus: map[string]string{
			"orchestrator":  "running",
			"total_active":  fmt.Sprintf("%d", stats.ActiveMemories),
			"total_fading":  fmt.Sprintf("%d", stats.FadingMemories),
		},
	}
	o.mu.Unlock()

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		o.log.Warn("failed to marshal health report", "error", err)
		return
	}

	dir := filepath.Dir(o.config.HealthReportPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		o.log.Warn("failed to create health report directory", "path", dir, "error", err)
		return
	}

	if err := os.WriteFile(o.config.HealthReportPath, data, 0644); err != nil {
		o.log.Warn("failed to write health report", "path", o.config.HealthReportPath, "error", err)
	}
}

// IsLLMHealthy reports whether the LLM backend is currently reachable.
func (o *Orchestrator) IsLLMHealthy() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.llmHealthy
}

// GetHealthReport generates a current health report.
func (o *Orchestrator) GetHealthReport(ctx context.Context) HealthReport {
	o.writeHealthReport()

	data, err := os.ReadFile(o.config.HealthReportPath)
	if err != nil {
		return HealthReport{Timestamp: time.Now()}
	}

	var report HealthReport
	_ = json.Unmarshal(data, &report)
	return report
}

func (o *Orchestrator) addWarning(msg string) {
	for _, w := range o.warnings {
		if w == msg {
			return // already present
		}
	}
	o.warnings = append(o.warnings, msg)
}

func (o *Orchestrator) removeWarning(msg string) {
	for i, w := range o.warnings {
		if w == msg {
			o.warnings = append(o.warnings[:i], o.warnings[i+1:]...)
			return
		}
	}
}
