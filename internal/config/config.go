package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure.
type Config struct {
	LLM           LLMConfig           `yaml:"llm"`
	Store         StoreConfig         `yaml:"store"`
	Memory        MemoryConfig        `yaml:"memory"`
	Perception    PerceptionConfig    `yaml:"perception"`
	Encoding      EncodingConfig      `yaml:"encoding"`
	Consolidation ConsolidationConfig `yaml:"consolidation"`
	Retrieval     RetrievalConfig     `yaml:"retrieval"`
	Metacognition MetacognitionConfig `yaml:"metacognition"`
	Dreaming      DreamingConfig      `yaml:"dreaming"`
	Episoding     EpisodingConfig     `yaml:"episoding"`
	Abstraction   AbstractionConfig   `yaml:"abstraction"`
	Orchestrator  OrchestratorConfig  `yaml:"orchestrator"`
	MCP           MCPConfig           `yaml:"mcp"`
	AgentSDK      AgentSDKConfig      `yaml:"agent_sdk"`
	Coaching      CoachingConfig      `yaml:"coaching"`
	API           APIConfig           `yaml:"api"`
	Web           WebConfig           `yaml:"web"`
	Logging       LoggingConfig       `yaml:"logging"`
}

// LLMConfig holds LLM provider settings.
type LLMConfig struct {
	Endpoint       string  `yaml:"endpoint"`
	ChatModel      string  `yaml:"chat_model"`
	EmbeddingModel string  `yaml:"embedding_model"`
	MaxTokens      int     `yaml:"max_tokens"`
	Temperature    float64 `yaml:"temperature"`
	TimeoutSec     int     `yaml:"timeout_sec"`
	MaxConcurrent  int     `yaml:"max_concurrent"` // max simultaneous LLM requests (0 = default 2)
}

// StoreConfig holds storage settings.
type StoreConfig struct {
	DBPath        string `yaml:"db_path"`
	JournalMode   string `yaml:"journal_mode"`
	BusyTimeoutMs int    `yaml:"busy_timeout_ms"` // SQLite busy timeout in milliseconds (default: 5000)
}

// MemoryConfig holds memory settings.
type MemoryConfig struct {
	MaxWorkingMemory int `yaml:"max_working_memory"`
}

// PerceptionConfig holds perception settings.
type PerceptionConfig struct {
	Enabled               bool                       `yaml:"enabled"`
	LLMGatingEnabled      bool                       `yaml:"llm_gating_enabled"`
	LearnedExclusionsPath string                     `yaml:"learned_exclusions_path"`
	Filesystem            FilesystemPerceptionConfig `yaml:"filesystem"`
	Git                   GitPerceptionConfig        `yaml:"git"`
	Terminal              TerminalPerceptionConfig   `yaml:"terminal"`
	Clipboard             ClipboardPerceptionConfig  `yaml:"clipboard"`
	Heuristics            HeuristicsConfig           `yaml:"heuristics"`
}

// FilesystemPerceptionConfig holds filesystem perception settings.
type FilesystemPerceptionConfig struct {
	Enabled            bool     `yaml:"enabled"`
	WatchDirs          []string `yaml:"watch_dirs"`
	ExcludePatterns    []string `yaml:"exclude_patterns"`
	MaxContentBytes    int      `yaml:"max_content_bytes"`
	MaxWatches         int      `yaml:"max_watches"`          // hard cap on inotify watches (Linux only, 0 = unlimited)
	ShallowDepth       int      `yaml:"shallow_depth"`        // inotify watch depth at startup (default: 3)
	PollIntervalSec    int      `yaml:"poll_interval_sec"`    // how often to scan cold directories (default: 45)
	PromotionThreshold int      `yaml:"promotion_threshold"`  // changes in poll window to promote to hot (default: 3)
	DemotionTimeoutMin int      `yaml:"demotion_timeout_min"` // minutes of inactivity before demotion (default: 30)
}

// GitPerceptionConfig holds git repository watching settings.
type GitPerceptionConfig struct {
	Enabled         bool `yaml:"enabled"`
	PollIntervalSec int  `yaml:"poll_interval_sec"` // how often to check each repo (default: 45)
	MaxRepoDepth    int  `yaml:"max_repo_depth"`    // how deep to scan for .git/ dirs (default: 3)
}

// TerminalPerceptionConfig holds terminal perception settings.
type TerminalPerceptionConfig struct {
	Enabled         bool     `yaml:"enabled"`
	Shell           string   `yaml:"shell"`
	PollIntervalSec int      `yaml:"poll_interval_sec"`
	ExcludePatterns []string `yaml:"exclude_patterns"`
}

// ClipboardPerceptionConfig holds clipboard perception settings.
type ClipboardPerceptionConfig struct {
	Enabled         bool `yaml:"enabled"`
	PollIntervalSec int  `yaml:"poll_interval_sec"`
	MaxContentBytes int  `yaml:"max_content_bytes"`
}

// HeuristicsConfig holds heuristics settings.
type HeuristicsConfig struct {
	MinContentLength   int `yaml:"min_content_length"`
	MaxContentLength   int `yaml:"max_content_length"`
	FrequencyThreshold int `yaml:"frequency_threshold"`
	FrequencyWindowMin int `yaml:"frequency_window_min"`
}

// EncodingConfig holds encoding settings.
type EncodingConfig struct {
	Enabled                  bool `yaml:"enabled"`
	UseLLM                   bool `yaml:"use_llm"`
	MaxLLMQueueSize          int  `yaml:"max_llm_queue_size"`
	MaxConcepts              int  `yaml:"max_concepts"`
	FindSimilarLimit         int  `yaml:"find_similar_limit"`
	EnableContextualEncoding bool `yaml:"enable_contextual_encoding"`
	ContextLookbackCount     int  `yaml:"context_lookback_count"`
	ContextSemanticCount     int  `yaml:"context_semantic_count"`
	MaxConcurrentEncodings   int  `yaml:"max_concurrent_encodings"`
	EnableLLMClassification  bool `yaml:"enable_llm_classification"`
	CompletionMaxTokens      int  `yaml:"completion_max_tokens"`
}

// ConsolidationConfig holds consolidation settings.
type ConsolidationConfig struct {
	Enabled             bool          `yaml:"enabled"`
	IntervalRaw         string        `yaml:"interval"`
	Interval            time.Duration `yaml:"-"`
	DecayRate           float64       `yaml:"decay_rate"`
	FadeThreshold       float64       `yaml:"fade_threshold"`
	ArchiveThreshold    float64       `yaml:"archive_threshold"`
	RetentionWindowRaw  string        `yaml:"retention_window"`
	RetentionWindow     time.Duration `yaml:"-"`
	MaxMemoriesPerCycle int           `yaml:"max_memories_per_cycle"`
	MaxMergesPerCycle   int           `yaml:"max_merges_per_cycle"`
	MinClusterSize      int           `yaml:"min_cluster_size"`
}

// RetrievalConfig holds retrieval settings.
type RetrievalConfig struct {
	MaxHops             int     `yaml:"max_hops"`
	ActivationThreshold float64 `yaml:"activation_threshold"`
	DecayFactor         float64 `yaml:"decay_factor"`
	MaxResults          int     `yaml:"max_results"`
	MaxToolCalls        int     `yaml:"max_tool_calls"`
	SynthesisMaxTokens  int     `yaml:"synthesis_max_tokens"`
	MergeAlpha          float64 `yaml:"merge_alpha"`
	DualHitBonus        float64 `yaml:"dual_hit_bonus"`
}

// MetacognitionConfig holds metacognition settings.
type MetacognitionConfig struct {
	Enabled     bool          `yaml:"enabled"`
	IntervalRaw string        `yaml:"interval"`
	Interval    time.Duration `yaml:"-"`
}

// DreamingConfig holds dreaming (memory replay) agent settings.
type DreamingConfig struct {
	Enabled                bool          `yaml:"enabled"`
	IntervalRaw            string        `yaml:"interval"`
	Interval               time.Duration `yaml:"-"`
	BatchSize              int           `yaml:"batch_size"`
	SalienceThreshold      float32       `yaml:"salience_threshold"`
	AssociationBoostFactor float32       `yaml:"association_boost_factor"`
	NoisePruneThreshold    float32       `yaml:"noise_prune_threshold"`
}

// EpisodingConfig configures the episoding agent.
type EpisodingConfig struct {
	Enabled              bool `yaml:"enabled"`
	EpisodeWindowSizeMin int  `yaml:"episode_window_size_min"`
	MinEventsPerEpisode  int  `yaml:"min_events_per_episode"`
}

// AbstractionConfig configures the abstraction agent (hierarchical knowledge).
type AbstractionConfig struct {
	Enabled     bool          `yaml:"enabled"`
	IntervalRaw string        `yaml:"interval"`
	Interval    time.Duration `yaml:"-"`
	MinStrength float32       `yaml:"min_strength"`  // minimum pattern strength to consider
	MaxLLMCalls int           `yaml:"max_llm_calls"` // budget per cycle
}

// OrchestratorConfig configures the autonomous orchestrator.
type OrchestratorConfig struct {
	Enabled             bool          `yaml:"enabled"`
	AdaptiveIntervals   bool          `yaml:"adaptive_intervals"`
	MaxDBSizeMB         int           `yaml:"max_db_size_mb"`
	SelfTestIntervalRaw string        `yaml:"self_test_interval"`
	SelfTestInterval    time.Duration `yaml:"-"`
	AutoRecovery        bool          `yaml:"auto_recovery"`
	MonitorIntervalRaw  string        `yaml:"monitor_interval"`
	MonitorInterval     time.Duration `yaml:"-"`
}

// MCPConfig holds MCP server settings.
type MCPConfig struct {
	Enabled bool `yaml:"enabled"`
}

// AgentSDKConfig holds Agent SDK integration settings.
type AgentSDKConfig struct {
	Enabled      bool   `yaml:"enabled"`
	EvolutionDir string `yaml:"evolution_dir"`
	WebPort      int    `yaml:"web_port"`   // Port for Python WebSocket server (default: 9998)
	PythonBin    string `yaml:"python_bin"` // Path to uv or python3 (default: auto-detect)
}

// APIConfig holds API server settings.
type APIConfig struct {
	Host              string `yaml:"host"`
	Port              int    `yaml:"port"`
	RequestTimeoutSec int    `yaml:"request_timeout_sec"`
}

// WebConfig holds web UI settings.
type WebConfig struct {
	Enabled bool `yaml:"enabled"`
}

// CoachingConfig holds settings for the Claude→local LLM coaching system.
type CoachingConfig struct {
	CoachingFile string `yaml:"coaching_file"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	File   string `yaml:"file"`
}

// Load reads and parses a YAML configuration file.
func Load(path string) (*Config, error) {
	// Expand ~ in path
	expanded, err := expandPath(path)
	if err != nil {
		return nil, fmt.Errorf("expanding config path: %w", err)
	}

	// Resolve to absolute so relative paths in config resolve against config dir
	absPath, err := filepath.Abs(expanded)
	if err != nil {
		return nil, fmt.Errorf("resolving config path: %w", err)
	}
	configDir := filepath.Dir(absPath)

	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config YAML: %w", err)
	}

	// Expand paths and parse durations; configDir used to resolve relative paths
	if err := cfg.process(configDir); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		LLM: LLMConfig{
			Endpoint:       "http://localhost:1234/v1",
			ChatModel:      "neural-chat",
			EmbeddingModel: "text-embedding-embeddinggemma-300m-qat",
			MaxTokens:      512,
			Temperature:    0.3,
			TimeoutSec:     120,
			MaxConcurrent:  2,
		},
		Store: StoreConfig{
			DBPath:        "~/.mnemonic/memory.db",
			JournalMode:   "wal",
			BusyTimeoutMs: 5000,
		},
		Memory: MemoryConfig{
			MaxWorkingMemory: 7,
		},
		Perception: PerceptionConfig{
			Enabled:               true,
			LLMGatingEnabled:      false,
			LearnedExclusionsPath: "~/.mnemonic/learned-exclusions.txt",
			Filesystem: FilesystemPerceptionConfig{
				Enabled: true,
				WatchDirs: []string{
					"~/Documents",
					"~/Projects",
				},
				ExcludePatterns: []string{
					".git/",
					"node_modules/",
					".DS_Store",
					"__pycache__/",
					"venv/",
					".venv/",
					"site-packages/",
				},
				MaxContentBytes:    102400,
				MaxWatches:         20000,
				ShallowDepth:       3,
				PollIntervalSec:    45,
				PromotionThreshold: 3,
				DemotionTimeoutMin: 30,
			},
			Git: GitPerceptionConfig{
				Enabled:         true,
				PollIntervalSec: 45,
				MaxRepoDepth:    3,
			},
			Terminal: TerminalPerceptionConfig{
				Enabled:         true,
				Shell:           "auto",
				PollIntervalSec: 10,
				ExcludePatterns: []string{
					"^cd ",
					"^ls ",
					"^pwd$",
				},
			},
			Clipboard: ClipboardPerceptionConfig{
				Enabled:         false,
				PollIntervalSec: 5,
				MaxContentBytes: 102400,
			},
			Heuristics: HeuristicsConfig{
				MinContentLength:   10,
				MaxContentLength:   100000,
				FrequencyThreshold: 5,
				FrequencyWindowMin: 10,
			},
		},
		Encoding: EncodingConfig{
			Enabled:                  true,
			UseLLM:                   true,
			MaxLLMQueueSize:          50,
			MaxConcepts:              5,
			FindSimilarLimit:         5,
			EnableContextualEncoding: true,
			ContextLookbackCount:     5,
			ContextSemanticCount:     3,
			MaxConcurrentEncodings:   1,
			EnableLLMClassification:  false,
			CompletionMaxTokens:      1024,
		},
		Consolidation: ConsolidationConfig{
			Enabled:             true,
			IntervalRaw:         "6h",
			Interval:            6 * time.Hour,
			DecayRate:           0.95,
			FadeThreshold:       0.3,
			ArchiveThreshold:    0.1,
			RetentionWindowRaw:  "90d",
			RetentionWindow:     90 * 24 * time.Hour,
			MaxMemoriesPerCycle: 100,
			MaxMergesPerCycle:   5,
			MinClusterSize:      3,
		},
		Retrieval: RetrievalConfig{
			MaxHops:             3,
			ActivationThreshold: 0.1,
			DecayFactor:         0.7,
			MaxResults:          7,
			MaxToolCalls:        5,
			SynthesisMaxTokens:  1024,
			MergeAlpha:          0.6,
			DualHitBonus:        0.15,
		},
		Metacognition: MetacognitionConfig{
			Enabled:     true,
			IntervalRaw: "24h",
			Interval:    24 * time.Hour,
		},
		Dreaming: DreamingConfig{
			Enabled:                true,
			IntervalRaw:            "3h",
			Interval:               3 * time.Hour,
			BatchSize:              20,
			SalienceThreshold:      0.3,
			AssociationBoostFactor: 1.15,
			NoisePruneThreshold:    0.15,
		},
		Episoding: EpisodingConfig{
			Enabled:              true,
			EpisodeWindowSizeMin: 10,
			MinEventsPerEpisode:  2,
		},
		Abstraction: AbstractionConfig{
			Enabled:     true,
			IntervalRaw: "6h",
			Interval:    6 * time.Hour,
			MinStrength: 0.4,
			MaxLLMCalls: 5,
		},
		Orchestrator: OrchestratorConfig{
			Enabled:             true,
			AdaptiveIntervals:   true,
			MaxDBSizeMB:         500,
			SelfTestIntervalRaw: "12h",
			SelfTestInterval:    12 * time.Hour,
			AutoRecovery:        true,
			MonitorIntervalRaw:  "5m",
			MonitorInterval:     5 * time.Minute,
		},
		MCP: MCPConfig{
			Enabled: true,
		},
		AgentSDK: AgentSDKConfig{
			Enabled:      false,
			EvolutionDir: "",
			WebPort:      9998,
		},
		Coaching: CoachingConfig{
			CoachingFile: "~/.mnemonic/coaching.yaml",
		},
		API: APIConfig{
			Host:              "127.0.0.1",
			Port:              9999,
			RequestTimeoutSec: 180,
		},
		Web: WebConfig{
			Enabled: true,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			File:   "~/.mnemonic/mnemonic.log",
		},
	}
}

// process expands paths and parses duration strings.
// configDir is the directory containing the config file, used to resolve relative paths.
func (c *Config) process(configDir string) error {
	var err error

	// Expand Store paths
	c.Store.DBPath, err = resolvePath(c.Store.DBPath, configDir)
	if err != nil {
		return fmt.Errorf("expanding store.db_path: %w", err)
	}

	// Expand Perception Filesystem watch dirs
	for i, dir := range c.Perception.Filesystem.WatchDirs {
		expanded, err := resolvePath(dir, configDir)
		if err != nil {
			return fmt.Errorf("expanding perception.filesystem.watch_dirs[%d]: %w", i, err)
		}
		c.Perception.Filesystem.WatchDirs[i] = expanded
	}

	// Expand Perception learned exclusions path
	if c.Perception.LearnedExclusionsPath != "" {
		c.Perception.LearnedExclusionsPath, err = resolvePath(c.Perception.LearnedExclusionsPath, configDir)
		if err != nil {
			return fmt.Errorf("expanding perception.learned_exclusions_path: %w", err)
		}
	}

	// Expand Logging file path
	c.Logging.File, err = resolvePath(c.Logging.File, configDir)
	if err != nil {
		return fmt.Errorf("expanding logging.file: %w", err)
	}

	// Parse duration strings from raw YAML values
	if c.Consolidation.IntervalRaw != "" {
		c.Consolidation.Interval, err = parseDurationString(c.Consolidation.IntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing consolidation.interval: %w", err)
		}
	}

	if c.Consolidation.RetentionWindowRaw != "" {
		c.Consolidation.RetentionWindow, err = parseDurationString(c.Consolidation.RetentionWindowRaw)
		if err != nil {
			return fmt.Errorf("parsing consolidation.retention_window: %w", err)
		}
	}

	if c.Metacognition.IntervalRaw != "" {
		c.Metacognition.Interval, err = parseDurationString(c.Metacognition.IntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing metacognition.interval: %w", err)
		}
	}

	if c.Dreaming.IntervalRaw != "" {
		c.Dreaming.Interval, err = parseDurationString(c.Dreaming.IntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing dreaming.interval: %w", err)
		}
	}

	if c.Abstraction.IntervalRaw != "" {
		c.Abstraction.Interval, err = parseDurationString(c.Abstraction.IntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing abstraction.interval: %w", err)
		}
	}

	if c.Orchestrator.SelfTestIntervalRaw != "" {
		c.Orchestrator.SelfTestInterval, err = parseDurationString(c.Orchestrator.SelfTestIntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing orchestrator.self_test_interval: %w", err)
		}
	}

	if c.Orchestrator.MonitorIntervalRaw != "" {
		c.Orchestrator.MonitorInterval, err = parseDurationString(c.Orchestrator.MonitorIntervalRaw)
		if err != nil {
			return fmt.Errorf("parsing orchestrator.monitor_interval: %w", err)
		}
	}

	// Expand AgentSDK evolution dir
	if c.AgentSDK.EvolutionDir != "" {
		c.AgentSDK.EvolutionDir, err = resolvePath(c.AgentSDK.EvolutionDir, configDir)
		if err != nil {
			return fmt.Errorf("expanding agent_sdk.evolution_dir: %w", err)
		}
	}

	// Expand Coaching file path
	if c.Coaching.CoachingFile != "" {
		c.Coaching.CoachingFile, err = resolvePath(c.Coaching.CoachingFile, configDir)
		if err != nil {
			return fmt.Errorf("expanding coaching.coaching_file: %w", err)
		}
	}

	// Set Episoding defaults
	if c.Episoding.EpisodeWindowSizeMin == 0 {
		c.Episoding.EpisodeWindowSizeMin = 10
	}
	if c.Episoding.MinEventsPerEpisode == 0 {
		c.Episoding.MinEventsPerEpisode = 2
	}

	// Set Encoding contextual defaults
	if c.Encoding.ContextLookbackCount == 0 {
		c.Encoding.ContextLookbackCount = 5
	}
	if c.Encoding.ContextSemanticCount == 0 {
		c.Encoding.ContextSemanticCount = 3
	}

	return nil
}

// Validate checks that required fields are set.
func (c *Config) Validate() error {
	if c.LLM.Endpoint == "" {
		return errors.New("llm.endpoint is required")
	}
	if c.LLM.ChatModel == "" {
		return errors.New("llm.chat_model is required")
	}
	if c.LLM.EmbeddingModel == "" {
		return errors.New("llm.embedding_model is required")
	}
	if c.Store.DBPath == "" {
		return errors.New("store.db_path is required")
	}
	if c.Store.BusyTimeoutMs < 0 {
		return errors.New("store.busy_timeout_ms must be >= 0")
	}

	// Verify db_path parent directory is writable
	dbDir := filepath.Dir(c.Store.DBPath)
	if info, err := os.Stat(dbDir); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("store.db_path parent %q exists but is not a directory", dbDir)
		}
		// Try to verify writability by creating a temp file
		tmp := filepath.Join(dbDir, ".mnemonic-write-test")
		if f, err := os.Create(tmp); err != nil {
			return fmt.Errorf("store.db_path parent %q is not writable: %w", dbDir, err)
		} else {
			_ = f.Close()
			_ = os.Remove(tmp)
		}
	}
	// If dir doesn't exist, EnsureDataDir will create it later

	// Warn about dangerous watch directories
	for _, dir := range c.Perception.Filesystem.WatchDirs {
		if dir == "/" {
			return fmt.Errorf("perception.filesystem.watch_dirs contains root directory — this will overwhelm the system")
		}
		home, _ := os.UserHomeDir()
		if home != "" && dir == home {
			return fmt.Errorf("perception.filesystem.watch_dirs contains home directory %q — use specific subdirectories instead (e.g. ~/Documents, ~/Projects)", home)
		}
	}
	if c.Memory.MaxWorkingMemory <= 0 {
		return errors.New("memory.max_working_memory must be > 0")
	}
	if c.LLM.MaxTokens <= 0 {
		return errors.New("llm.max_tokens must be > 0")
	}
	if c.LLM.Temperature < 0 || c.LLM.Temperature > 2 {
		return errors.New("llm.temperature must be between 0 and 2")
	}
	if c.API.Port <= 0 || c.API.Port > 65535 {
		return errors.New("api.port must be between 1 and 65535")
	}
	if c.Consolidation.DecayRate <= 0 || c.Consolidation.DecayRate > 1 {
		return errors.New("consolidation.decay_rate must be between 0 and 1")
	}
	if c.Consolidation.FadeThreshold < 0 || c.Consolidation.FadeThreshold > 1 {
		return errors.New("consolidation.fade_threshold must be between 0 and 1")
	}
	if c.Consolidation.ArchiveThreshold < 0 || c.Consolidation.ArchiveThreshold > 1 {
		return errors.New("consolidation.archive_threshold must be between 0 and 1")
	}
	if c.Retrieval.ActivationThreshold < 0 || c.Retrieval.ActivationThreshold > 1 {
		return errors.New("retrieval.activation_threshold must be between 0 and 1")
	}
	if c.Retrieval.DecayFactor <= 0 || c.Retrieval.DecayFactor > 1 {
		return errors.New("retrieval.decay_factor must be between 0 and 1")
	}
	return nil
}

// expandPath expands ~ to the user's home directory.
func expandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	if path == "~" {
		return home, nil
	}

	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:]), nil
	}

	return path, nil
}

// resolvePath expands ~ to the user's home directory, and resolves relative
// paths against configDir (the directory containing the config file).
// Absolute paths are returned as-is.
func resolvePath(path, configDir string) (string, error) {
	if strings.HasPrefix(path, "~") {
		return expandPath(path)
	}
	if !filepath.IsAbs(path) {
		return filepath.Join(configDir, path), nil
	}
	return path, nil
}

// WarnPermissions checks if the config file at path has overly permissive file
// permissions. Returns a warning message or empty string.
func WarnPermissions(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	mode := info.Mode().Perm()
	// Warn if world-readable (others have read access)
	if mode&0004 != 0 {
		return fmt.Sprintf("config file %s is world-readable (mode %04o) — consider chmod 600", path, mode)
	}
	return ""
}

// EnsureDataDir creates the parent directory of the database path if it does not exist.
// Safe to call multiple times. Typically called before opening the database.
func (c *Config) EnsureDataDir() error {
	dbDir := filepath.Dir(c.Store.DBPath)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return fmt.Errorf("creating data directory %q: %w", dbDir, err)
	}
	return nil
}

// parseDurationString parses duration strings like "6h", "24h", "90d".
func parseDurationString(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)

	// Handle standard time.ParseDuration formats
	if dur, err := time.ParseDuration(s); err == nil {
		return dur, nil
	}

	// Handle custom formats like "6h", "24h", "90d"
	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)(h|d|w)$`)
	matches := re.FindStringSubmatch(s)
	if len(matches) == 0 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	numStr := matches[1]
	unit := matches[2]

	numVal, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing duration %s: %w", s, err)
	}

	switch unit {
	case "h":
		return time.Duration(numVal * float64(time.Hour)), nil
	case "d":
		return time.Duration(numVal * 24 * float64(time.Hour)), nil
	case "w":
		return time.Duration(numVal * 7 * 24 * float64(time.Hour)), nil
	default:
		return 0, fmt.Errorf("unknown duration unit in %s", s)
	}
}
