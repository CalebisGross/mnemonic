package reactor

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/events"
	"gopkg.in/yaml.v3"
)

// ChainDeps holds agent references and callbacks needed to build chains.
type ChainDeps struct {
	ConsolidationTrigger chan<- struct{}
	AbstractionTrigger   chan<- struct{}
	MetacognitionTrigger chan<- struct{}
	DreamingTrigger      chan<- struct{}
	IncrementAutonomous  func()
	MaxDBSizeMB          int
	CooldownOverrides    map[string]time.Duration // chain ID -> cooldown override
	Logger               *slog.Logger
}

// cooldown returns the override duration for a chain if set, otherwise the default.
func (d ChainDeps) cooldown(chainID string, defaultDuration time.Duration) time.Duration {
	if d.CooldownOverrides != nil {
		if override, ok := d.CooldownOverrides[chainID]; ok && override > 0 {
			return override
		}
	}
	return defaultDuration
}

// NewChainRegistry creates a registry with all built-in reactive chains.
func NewChainRegistry(deps ChainDeps) []*Chain {
	log := deps.Logger
	var chains []*Chain

	// Chain 1: Metacognition → Consolidation (high dead ratio)
	chains = append(chains, &Chain{
		ID:          "meta_consolidation_on_dead_ratio",
		Name:        "Metacognition: Consolidation on High Dead Ratio",
		Description: "Request consolidation when dead memory ratio is warning/critical",
		Trigger:     EventTypeMatcher{EventType: events.TypeMetaCycleCompleted},
		TriggerType: events.TypeMetaCycleCompleted,
		Conditions: []Condition{
			&ObservationSeverityCondition{
				ObservationType: "recall_effectiveness",
				MinSeverity:     "warning",
			},
			&CooldownCondition{
				ChainID:  "meta_consolidation_on_dead_ratio",
				Duration: deps.cooldown("meta_consolidation_on_dead_ratio", 30*time.Minute),
			},
		},
		Actions: []Action{
			&PublishEventAction{
				EventFactory: func() events.Event {
					return events.ConsolidationStarted{Ts: time.Now()}
				},
				Log: log,
			},
			&LogMetaObservationAction{
				ActionName:  "requested_consolidation",
				TriggerName: "high_dead_ratio",
				Log:         log,
			},
		},
		Cooldown: deps.cooldown("meta_consolidation_on_dead_ratio", 30*time.Minute),
		Priority: 10,
		Enabled:  true,
	})

	// Chain 2: Orchestrator → Consolidation (DB size threshold)
	chains = append(chains, &Chain{
		ID:          "orch_consolidation_on_db_size",
		Name:        "Orchestrator: Consolidation on DB Size Threshold",
		Description: "Request consolidation when database exceeds size limit",
		Trigger:     EventTypeMatcher{EventType: events.TypeSystemHealth},
		TriggerType: events.TypeSystemHealth,
		Conditions: []Condition{
			&DBSizeCondition{MaxSizeMB: deps.MaxDBSizeMB},
			&CooldownCondition{
				ChainID:  "orch_consolidation_on_db_size",
				Duration: deps.cooldown("orch_consolidation_on_db_size", 1*time.Hour),
			},
		},
		Actions: []Action{
			&PublishEventAction{
				EventFactory: func() events.Event {
					return events.ConsolidationStarted{Ts: time.Now()}
				},
				Log: log,
			},
			&IncrementCounterAction{
				CounterName: "orchestrator_autonomous",
				Increment:   deps.IncrementAutonomous,
			},
		},
		Cooldown: deps.cooldown("orch_consolidation_on_db_size", 1*time.Hour),
		Priority: 5,
		Enabled:  true,
	})

	// Chain 3: ConsolidationStarted → Consolidation Agent trigger
	if deps.ConsolidationTrigger != nil {
		chains = append(chains, &Chain{
			ID:          "consolidation_on_request",
			Name:        "Consolidation: Execute On-Demand",
			Description: "Send trigger signal to consolidation agent when consolidation is requested",
			Trigger:     EventTypeMatcher{EventType: events.TypeConsolidationStarted},
			TriggerType: events.TypeConsolidationStarted,
			Conditions: []Condition{
				&CooldownCondition{
					ChainID:  "consolidation_on_request",
					Duration: deps.cooldown("consolidation_on_request", 5*time.Minute),
				},
			},
			Actions: []Action{
				&SendToChannelAction{
					ChannelName: "consolidation_trigger",
					Channel:     deps.ConsolidationTrigger,
					Log:         log,
				},
			},
			Cooldown: deps.cooldown("consolidation_on_request", 5*time.Minute),
			Priority: 100,
			Enabled:  true,
		})
	}

	// Chain 4: PatternDiscovered → Abstraction Agent trigger
	if deps.AbstractionTrigger != nil {
		chains = append(chains, &Chain{
			ID:          "abstraction_on_pattern",
			Name:        "Abstraction: Execute On Pattern Discovery",
			Description: "Trigger abstraction cycle when a new pattern is discovered",
			Trigger:     EventTypeMatcher{EventType: events.TypePatternDiscovered},
			TriggerType: events.TypePatternDiscovered,
			Conditions:  []Condition{},
			Actions: []Action{
				&SendToChannelAction{
					ChannelName: "abstraction_trigger",
					Channel:     deps.AbstractionTrigger,
					Log:         log,
				},
			},
			Cooldown: 0,
			Priority: 50,
			Enabled:  true,
		})
	}

	// Chain 5: ConsolidationCompleted → Metacognition Agent trigger
	if deps.MetacognitionTrigger != nil {
		chains = append(chains, &Chain{
			ID:          "meta_on_consolidation_completed",
			Name:        "Metacognition: Execute After Consolidation",
			Description: "Trigger metacognition cycle when consolidation completes",
			Trigger:     EventTypeMatcher{EventType: events.TypeConsolidationCompleted},
			TriggerType: events.TypeConsolidationCompleted,
			Conditions: []Condition{
				&CooldownCondition{
					ChainID:  "meta_on_consolidation_completed",
					Duration: deps.cooldown("meta_on_consolidation_completed", 30*time.Minute),
				},
			},
			Actions: []Action{
				&SendToChannelAction{
					ChannelName: "metacognition_trigger",
					Channel:     deps.MetacognitionTrigger,
					Log:         log,
				},
			},
			Cooldown: deps.cooldown("meta_on_consolidation_completed", 30*time.Minute),
			Priority: 40,
			Enabled:  true,
		})
	}

	// Chain 6: EpisodeClosed → Dreaming Agent trigger
	if deps.DreamingTrigger != nil {
		chains = append(chains, &Chain{
			ID:          "dreaming_on_episode_closed",
			Name:        "Dreaming: Execute After Episode Closes",
			Description: "Trigger dream cycle when an episode is closed",
			Trigger:     EventTypeMatcher{EventType: events.TypeEpisodeClosed},
			TriggerType: events.TypeEpisodeClosed,
			Conditions: []Condition{
				&CooldownCondition{
					ChainID:  "dreaming_on_episode_closed",
					Duration: deps.cooldown("dreaming_on_episode_closed", 10*time.Minute),
				},
			},
			Actions: []Action{
				&SendToChannelAction{
					ChannelName: "dreaming_trigger",
					Channel:     deps.DreamingTrigger,
					Log:         log,
				},
			},
			Cooldown: deps.cooldown("dreaming_on_episode_closed", 10*time.Minute),
			Priority: 30,
			Enabled:  true,
		})
	}

	return chains
}

// ChainConfig is the YAML-serializable representation of a chain.
type ChainConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	TriggerType string `yaml:"trigger_type"`
	Cooldown    string `yaml:"cooldown"`
	Priority    int    `yaml:"priority"`
	Enabled     bool   `yaml:"enabled"`
}

// ChainsFileConfig is the top-level YAML config structure.
type ChainsFileConfig struct {
	Chains []ChainConfig `yaml:"chains"`
}

// LoadChainsFromFile loads chain configs from a YAML file.
// Returns parsed configs that can be used to override built-in chain settings.
func LoadChainsFromFile(path string) ([]ChainConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read chains config: %w", err)
	}

	var cfg ChainsFileConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse chains config: %w", err)
	}

	return cfg.Chains, nil
}
