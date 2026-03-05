package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/appsprout/mnemonic/internal/config"
	"github.com/appsprout/mnemonic/internal/daemon"
	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/logger"
	"github.com/appsprout/mnemonic/internal/store/sqlite"
	"github.com/appsprout/mnemonic/internal/watcher"

	"github.com/appsprout/mnemonic/internal/agent/abstraction"
	"github.com/appsprout/mnemonic/internal/agent/consolidation"
	"github.com/appsprout/mnemonic/internal/agent/dreaming"
	"github.com/appsprout/mnemonic/internal/agent/encoding"
	"github.com/appsprout/mnemonic/internal/agent/episoding"
	"github.com/appsprout/mnemonic/internal/agent/metacognition"
	"github.com/appsprout/mnemonic/internal/agent/orchestrator"
	"github.com/appsprout/mnemonic/internal/agent/perception"
	"github.com/appsprout/mnemonic/internal/agent/reactor"
	"github.com/appsprout/mnemonic/internal/agent/retrieval"
	"github.com/appsprout/mnemonic/internal/api"
	"github.com/appsprout/mnemonic/internal/backup"
	"github.com/appsprout/mnemonic/internal/mcp"
	"github.com/appsprout/mnemonic/internal/store"

	clipwatcher "github.com/appsprout/mnemonic/internal/watcher/clipboard"
	fswatcher "github.com/appsprout/mnemonic/internal/watcher/filesystem"
	gitwatcher "github.com/appsprout/mnemonic/internal/watcher/git"
	termwatcher "github.com/appsprout/mnemonic/internal/watcher/terminal"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var Version = "dev"

const (
	defaultConfigPath = "config.yaml"
	dataDir           = "~/.mnemonic"
	bufferSize        = 1000
)

// ANSI color codes for terminal output
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[90m"
	colorBold   = "\033[1m"
)

func main() {
	// Parse global flags
	configPath := flag.String("config", defaultConfigPath, "path to config.yaml")
	flag.Parse()

	// Get subcommand from remaining arguments
	args := flag.Args()
	subcommand := "serve"
	if len(args) > 0 {
		subcommand = args[0]
	}

	// Handle help
	if subcommand == "--help" || subcommand == "-h" || subcommand == "help" {
		printUsage()
		os.Exit(0)
	}

	// Route to appropriate subcommand
	switch subcommand {
	case "serve":
		serveCommand(*configPath)
	case "start":
		startCommand(*configPath)
	case "stop":
		stopCommand()
	case "restart":
		restartCommand(*configPath)
	case "ingest":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'ingest' requires directory argument\nUsage: mnemonic ingest <directory> [--dry-run] [--project NAME]\n")
			os.Exit(1)
		}
		ingestCommand(*configPath, args[1:])
	case "remember":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'remember' requires text argument\n")
			os.Exit(1)
		}
		rememberCommand(*configPath, args[1])
	case "recall":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'recall' requires query argument\n")
			os.Exit(1)
		}
		recallCommand(*configPath, args[1])
	case "status":
		statusCommand(*configPath)
	case "consolidate":
		consolidateCommand(*configPath)
	case "watch":
		watchCommand(*configPath)
	case "install":
		installCommand(*configPath)
	case "uninstall":
		uninstallCommand()
	case "export":
		exportCommand(*configPath, args)
	case "import":
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: 'import' requires file path argument\n")
			os.Exit(1)
		}
		importCommand(*configPath, args[1], args)
	case "backup":
		backupCommand(*configPath)
	case "insights":
		insightsCommand(*configPath)
	case "meta-cycle":
		metaCycleCommand(*configPath)
	case "dream-cycle":
		dreamCycleCommand(*configPath)
	case "purge":
		purgeCommand(*configPath)
	case "cleanup":
		cleanupCommand(*configPath, args)
	case "mcp":
		mcpCommand(*configPath)
	case "autopilot":
		autopilotCommand(*configPath)
	case "version":
		fmt.Printf("mnemonic v%s\n", Version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcommand)
		printUsage()
		os.Exit(1)
	}
}

// ============================================================================
// Daemon Management Commands (start / stop / restart)
// ============================================================================

// startCommand launches the mnemonic daemon in the background.
func startCommand(configPath string) {
	svc := daemon.NewServiceManager()

	// If platform service is installed, use it
	if svc.IsInstalled() {
		if running, pid := svc.IsRunning(); running {
			fmt.Printf("Mnemonic is already running (%s, PID %d)\n", svc.ServiceName(), pid)
			os.Exit(1)
		}
		fmt.Printf("Starting mnemonic service...\n")
		if err := svc.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting service: %v\n", err)
			os.Exit(1)
		}
		// Wait and check if it started
		time.Sleep(2 * time.Second)
		if running, pid := svc.IsRunning(); running {
			cfg, _ := config.Load(configPath)
			fmt.Printf("%sMnemonic started%s (%s, PID %d)\n", colorGreen, colorReset, svc.ServiceName(), pid)
			if cfg != nil {
				fmt.Printf("  Dashboard: http://%s:%d\n", cfg.API.Host, cfg.API.Port)
			}
			fmt.Printf("  Logs:      %s\n", daemon.LogPath())
		} else {
			fmt.Printf("%sWarning:%s Service started but process not running yet.\n", colorYellow, colorReset)
			fmt.Printf("  Check logs: %s\n", daemon.LogPath())
		}
		return
	}

	// Fall back to PID-file-based daemon start
	if running, pid := daemon.IsRunning(); running {
		fmt.Printf("Mnemonic is already running (PID %d)\n", pid)
		os.Exit(1)
	}

	// Validate config can be loaded before starting
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Resolve to absolute config path (so daemon finds it after detach)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	// Get our binary path
	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding executable: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Starting mnemonic daemon...\n")

	pid, err := daemon.Start(execPath, absConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting daemon: %v\n", err)
		os.Exit(1)
	}

	// Wait briefly and verify daemon is healthy via API
	time.Sleep(2 * time.Second)
	apiURL := fmt.Sprintf("http://%s:%d/api/v1/health", cfg.API.Host, cfg.API.Port)
	healthy := false
	for i := 0; i < 3; i++ {
		resp, err := http.Get(apiURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				healthy = true
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if healthy {
		fmt.Printf("%sMnemonic started%s (PID %d)\n", colorGreen, colorReset, pid)
		fmt.Printf("  Dashboard: http://%s:%d\n", cfg.API.Host, cfg.API.Port)
		fmt.Printf("  Logs:      %s\n", daemon.LogPath())
		fmt.Printf("  PID file:  %s\n", daemon.PIDFilePath())
	} else {
		fmt.Printf("%sWarning:%s Daemon started (PID %d) but health check failed.\n", colorYellow, colorReset, pid)
		fmt.Printf("  Check logs: %s\n", daemon.LogPath())
	}
}

// stopCommand stops the running mnemonic daemon.
func stopCommand() {
	svc := daemon.NewServiceManager()

	// Check platform service first
	if svc.IsInstalled() {
		if running, pid := svc.IsRunning(); running {
			fmt.Printf("Stopping mnemonic service (PID %d)...\n", pid)
			if err := svc.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping service: %v\n", err)
				os.Exit(1)
			}
			// Wait for process to actually exit
			time.Sleep(2 * time.Second)
			fmt.Printf("%sMnemonic stopped.%s\n", colorGreen, colorReset)
			return
		}
	}

	// Fall back to PID file
	running, pid := daemon.IsRunning()
	if !running {
		fmt.Println("Mnemonic is not running.")
		os.Exit(0)
	}

	fmt.Printf("Stopping mnemonic daemon (PID %d)...\n", pid)

	if err := daemon.Stop(); err != nil {
		fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sMnemonic stopped.%s\n", colorGreen, colorReset)
}

// restartCommand stops and starts the mnemonic daemon.
func restartCommand(configPath string) {
	svc := daemon.NewServiceManager()

	// Check platform service first
	if svc.IsInstalled() {
		if running, pid := svc.IsRunning(); running {
			fmt.Printf("Stopping mnemonic service (PID %d)...\n", pid)
			if err := svc.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "Error stopping service: %v\n", err)
				os.Exit(1)
			}
			time.Sleep(2 * time.Second)
		}
		startCommand(configPath)
		return
	}

	// Fall back to PID file
	if running, pid := daemon.IsRunning(); running {
		fmt.Printf("Stopping mnemonic daemon (PID %d)...\n", pid)
		if err := daemon.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Error stopping daemon: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(1 * time.Second)
	}

	startCommand(configPath)
}

// ============================================================================
// Watch Command (live event tail)
// ============================================================================

// watchCommand connects to the daemon's WebSocket and streams live events.
func watchCommand(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	wsURL := fmt.Sprintf("ws://%s:%d/ws", cfg.API.Host, cfg.API.Port)

	fmt.Printf("%sMnemonic Live Events%s — connecting to %s\n", colorBold, colorReset, wsURL)
	fmt.Printf("Press Ctrl+C to stop.\n\n")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to daemon WebSocket: %v\n", err)
		fmt.Fprintf(os.Stderr, "Is the daemon running? Try: mnemonic start\n")
		os.Exit(1)
	}
	defer conn.Close()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Printf("\n%sStopping event watch.%s\n", colorGray, colorReset)
		conn.Close()
		os.Exit(0)
	}()

	// Read and display events
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				fmt.Println("Connection closed.")
			} else {
				fmt.Fprintf(os.Stderr, "\nWebSocket disconnected: %v\n", err)
			}
			return
		}

		formatWatchEvent(message)
	}
}

// formatWatchEvent formats and prints a WebSocket event with colors.
func formatWatchEvent(data []byte) {
	var evt map[string]interface{}
	if err := json.Unmarshal(data, &evt); err != nil {
		// Raw text event
		ts := time.Now().Format("15:04:05")
		fmt.Printf("%s%s%s %s\n", colorGray, ts, colorReset, string(data))
		return
	}

	eventType, _ := evt["type"].(string)
	ts := time.Now().Format("15:04:05")

	switch eventType {
	case "raw_memory_created":
		source, _ := evt["source"].(string)
		id, _ := evt["id"].(string)
		shortID := truncID(id)
		fmt.Printf("%s%s%s %s▶ PERCEIVED%s [%s] %s\n",
			colorGray, ts, colorReset, colorCyan, colorReset, source, shortID)

	case "memory_encoded":
		id, _ := evt["id"].(string)
		shortID := truncID(id)
		fmt.Printf("%s%s%s %s▶ ENCODED%s   %s\n",
			colorGray, ts, colorReset, colorGreen, colorReset, shortID)

	case "consolidation_completed":
		processed, _ := evt["memories_processed"].(float64)
		decayed, _ := evt["memories_decayed"].(float64)
		merged, _ := evt["merged_clusters"].(float64)
		pruned, _ := evt["associations_pruned"].(float64)
		durationMs, _ := evt["duration_ms"].(float64)
		fmt.Printf("%s%s%s %s▶ CONSOLIDATED%s  processed=%d decayed=%d merged=%d pruned=%d (%dms)\n",
			colorGray, ts, colorReset, colorYellow, colorReset,
			int(processed), int(decayed), int(merged), int(pruned), int(durationMs))

	case "query_executed":
		query, _ := evt["query"].(string)
		results, _ := evt["result_count"].(float64)
		took, _ := evt["took_ms"].(float64)
		fmt.Printf("%s%s%s %s▶ QUERY%s      \"%s\" → %d results (%dms)\n",
			colorGray, ts, colorReset, colorBlue, colorReset,
			query, int(results), int(took))

	case "dream_cycle_completed":
		replayed, _ := evt["memories_replayed"].(float64)
		strengthened, _ := evt["associations_strengthened"].(float64)
		newAssoc, _ := evt["new_associations_created"].(float64)
		demoted, _ := evt["noisy_memories_demoted"].(float64)
		durationMs, _ := evt["duration_ms"].(float64)
		fmt.Printf("%s%s%s %s▶ DREAMED%s    replayed=%d strengthened=%d new_assoc=%d demoted=%d (%dms)\n",
			colorGray, ts, colorReset, colorCyan, colorReset,
			int(replayed), int(strengthened), int(newAssoc), int(demoted), int(durationMs))

	case "meta_cycle_completed":
		observations, _ := evt["observations_logged"].(float64)
		fmt.Printf("%s%s%s %s▶ META%s       observations=%d\n",
			colorGray, ts, colorReset, colorCyan, colorReset, int(observations))

	default:
		// Generic event
		fmt.Printf("%s%s%s %s▶ %s%s  %s\n",
			colorGray, ts, colorReset, colorGray, eventType, colorReset,
			string(data))
	}
}

// truncID shortens a UUID for display.
func truncID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ============================================================================
// Enhanced Status Command
// ============================================================================

// statusCommand displays comprehensive system status.
func statusCommand(configPath string) {
	svc := daemon.NewServiceManager()

	cfg, err := config.Load(configPath)
	if err != nil {
		// Even without config, show daemon state
		fmt.Printf("%sMnemonic v%s Status%s\n\n", colorBold, Version, colorReset)
		if svcRunning, svcPid := svc.IsRunning(); svcRunning {
			fmt.Printf("  Daemon:  %srunning%s (%s, PID %d)\n", colorGreen, colorReset, svc.ServiceName(), svcPid)
		} else if running, pid := daemon.IsRunning(); running {
			fmt.Printf("  Daemon:  %srunning%s (PID %d)\n", colorGreen, colorReset, pid)
		} else {
			fmt.Printf("  Daemon:  %sstopped%s\n", colorRed, colorReset)
		}
		fmt.Fprintf(os.Stderr, "  (Config error: %v)\n", err)
		return
	}

	fmt.Printf("%sMnemonic v%s Status%s\n\n", colorBold, Version, colorReset)

	// Daemon state — check platform service first, then PID file
	running := false
	pid := 0
	mode := ""
	if svcRunning, svcPid := svc.IsRunning(); svcRunning {
		running, pid, mode = true, svcPid, fmt.Sprintf(" (%s)", svc.ServiceName())
	} else if pidRunning, pidPid := daemon.IsRunning(); pidRunning {
		running, pid = true, pidPid
	}
	if running {
		fmt.Printf("  Daemon:     %srunning%s%s (PID %d)\n", colorGreen, colorReset, mode, pid)
	} else {
		fmt.Printf("  Daemon:     %sstopped%s\n", colorRed, colorReset)
	}

	// Try to get live status from the API
	apiBase := fmt.Sprintf("http://%s:%d/api/v1", cfg.API.Host, cfg.API.Port)
	apiReachable := false

	// Health check
	healthResp, err := http.Get(apiBase + "/health")
	if err == nil {
		defer healthResp.Body.Close()
		if healthResp.StatusCode == http.StatusOK {
			apiReachable = true
			var health map[string]interface{}
			if json.NewDecoder(healthResp.Body).Decode(&health) == nil {
				llmStatus, _ := health["llm"].(string)
				storeStatus, _ := health["store"].(string)

				llmColor := colorGreen
				if llmStatus != "ok" {
					llmColor = colorRed
				}
				storeColor := colorGreen
				if storeStatus != "ok" {
					storeColor = colorRed
				}

				fmt.Printf("  API:        %slistening%s on %s:%d\n", colorGreen, colorReset, cfg.API.Host, cfg.API.Port)
				fmt.Printf("  LLM:        %s%s%s (%s)\n", llmColor, llmStatus, colorReset, cfg.LLM.ChatModel)
				fmt.Printf("  Store:      %s%s%s\n", storeColor, storeStatus, colorReset)
			}
		}
	}

	if !apiReachable {
		fmt.Printf("  API:        %sunreachable%s\n", colorRed, colorReset)
	}

	// Memory stats — from API if available, else direct DB
	fmt.Printf("\n  %sMemory Store%s\n", colorBold, colorReset)

	if apiReachable {
		statsResp, err := http.Get(apiBase + "/stats")
		if err == nil {
			defer statsResp.Body.Close()
			var data map[string]interface{}
			if json.NewDecoder(statsResp.Body).Decode(&data) == nil {
				s, _ := data["store"].(map[string]interface{})
				if s == nil {
					s = data
				}
				total := intVal(s, "total_memories")
				active := intVal(s, "active_memories")
				fading := intVal(s, "fading_memories")
				archived := intVal(s, "archived_memories")
				merged := intVal(s, "merged_memories")
				assoc := intVal(s, "total_associations")
				dbSize := intVal(s, "storage_size_bytes")

				fmt.Printf("    Total:          %d\n", total)
				fmt.Printf("    Active:         %s%d%s\n", colorGreen, active, colorReset)
				fmt.Printf("    Fading:         %s%d%s\n", colorYellow, fading, colorReset)
				fmt.Printf("    Archived:       %s%d%s\n", colorGray, archived, colorReset)
				fmt.Printf("    Merged:         %d\n", merged)
				fmt.Printf("    Associations:   %d\n", assoc)
				fmt.Printf("    DB size:        %.1f KB\n", float64(dbSize)/1024)
			}
		}
	} else {
		// Fall back to direct DB access
		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath)
		if err == nil {
			defer db.Close()
			ctx := context.Background()
			stats, err := db.GetStatistics(ctx)
			if err == nil {
				fmt.Printf("    Total:          %d\n", stats.TotalMemories)
				fmt.Printf("    Active:         %s%d%s\n", colorGreen, stats.ActiveMemories, colorReset)
				fmt.Printf("    Fading:         %s%d%s\n", colorYellow, stats.FadingMemories, colorReset)
				fmt.Printf("    Archived:       %s%d%s\n", colorGray, stats.ArchivedMemories, colorReset)
				fmt.Printf("    Merged:         %d\n", stats.MergedMemories)
				fmt.Printf("    Associations:   %d\n", stats.TotalAssociations)
				fmt.Printf("    DB size:        %.1f KB\n", float64(stats.StorageSizeBytes)/1024)
			}
		}
	}

	// Consolidation status — check last consolidation from DB
	fmt.Printf("\n  %sConsolidation%s\n", colorBold, colorReset)
	if cfg.Consolidation.Enabled {
		fmt.Printf("    Enabled:        yes (every %s)\n", cfg.Consolidation.IntervalRaw)
		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath)
		if err == nil {
			defer db.Close()
			lastConsolidation := getLastConsolidation(db)
			if lastConsolidation != "" {
				fmt.Printf("    Last run:       %s\n", lastConsolidation)
			} else {
				fmt.Printf("    Last run:       %snever%s\n", colorGray, colorReset)
			}
		}
	} else {
		fmt.Printf("    Enabled:        no\n")
	}

	// Perception config
	fmt.Printf("\n  %sPerception%s\n", colorBold, colorReset)
	if cfg.Perception.Enabled {
		if cfg.Perception.Filesystem.Enabled {
			fmt.Printf("    Filesystem:     %senabled%s (%d dirs)\n", colorGreen, colorReset, len(cfg.Perception.Filesystem.WatchDirs))
		} else {
			fmt.Printf("    Filesystem:     %sdisabled%s\n", colorGray, colorReset)
		}
		if cfg.Perception.Terminal.Enabled {
			fmt.Printf("    Terminal:       %senabled%s (poll %ds)\n", colorGreen, colorReset, cfg.Perception.Terminal.PollIntervalSec)
		} else {
			fmt.Printf("    Terminal:       %sdisabled%s\n", colorGray, colorReset)
		}
		if cfg.Perception.Clipboard.Enabled {
			fmt.Printf("    Clipboard:      %senabled%s\n", colorGreen, colorReset)
		} else {
			fmt.Printf("    Clipboard:      %sdisabled%s\n", colorGray, colorReset)
		}
	} else {
		fmt.Printf("    All perception: %sdisabled%s\n", colorGray, colorReset)
	}

	// Paths
	fmt.Printf("\n  %sPaths%s\n", colorBold, colorReset)
	fmt.Printf("    Config:         %s\n", configPath)
	fmt.Printf("    Database:       %s\n", cfg.Store.DBPath)
	fmt.Printf("    Log:            %s\n", daemon.LogPath())
	fmt.Printf("    PID:            %s\n", daemon.PIDFilePath())
	fmt.Printf("    Dashboard:      http://%s:%d\n", cfg.API.Host, cfg.API.Port)
	fmt.Println()
}

// intVal safely extracts an int from a JSON map.
func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}

// getLastConsolidation queries for the last consolidation timestamp.
func getLastConsolidation(db *sqlite.SQLiteStore) string {
	ctx := context.Background()
	record, err := db.GetLastConsolidation(ctx)
	if err != nil {
		return ""
	}
	if record.ID == "" {
		return ""
	}
	ago := time.Since(record.EndTime).Round(time.Minute)
	return fmt.Sprintf("%s (%s ago, %d memories, %dms)", record.EndTime.Format("Jan 2 15:04"), formatDuration(ago), record.MemoriesProcessed, record.DurationMs)
}

// formatDuration formats a duration as human-readable.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		mins := int(d.Minutes())
		return fmt.Sprintf("%dm", mins)
	}
	if d < 24*time.Hour {
		hours := int(d.Hours())
		return fmt.Sprintf("%dh", hours)
	}
	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

// ============================================================================
// Install / Uninstall (platform service)
// ============================================================================

// installCommand registers mnemonic as a platform service (launchd on macOS, systemd on Linux).
func installCommand(configPath string) {
	svc := daemon.NewServiceManager()

	// Validate config
	_, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Resolve paths
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	execPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding executable: %v\n", err)
		os.Exit(1)
	}

	if err := svc.Install(execPath, absConfigPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error installing service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sService installed (%s).%s\n\n", colorGreen, svc.ServiceName(), colorReset)
	fmt.Printf("  Binary:  %s\n", execPath)
	fmt.Printf("  Config:  %s\n", absConfigPath)
	fmt.Printf("\nMnemonic will now start automatically on login.\n")
	fmt.Printf("To start immediately:\n")
	fmt.Printf("  mnemonic start\n\n")
	fmt.Printf("To check status:\n")
	fmt.Printf("  mnemonic status\n\n")
	fmt.Printf("To uninstall:\n")
	fmt.Printf("  mnemonic uninstall\n")
}

// uninstallCommand removes the platform service registration.
func uninstallCommand() {
	svc := daemon.NewServiceManager()

	if err := svc.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "Error uninstalling service: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sService uninstalled (%s).%s\n", colorGreen, svc.ServiceName(), colorReset)
	fmt.Printf("Mnemonic will no longer start automatically on login.\n")
}

// ============================================================================
// Serve Command (the actual daemon)
// ============================================================================

// startAgentWebServer starts the Python WebSocket agent server as a child process.
// Returns the started Cmd for later cleanup, or nil if disabled/failed.
func startAgentWebServer(cfg *config.Config, log *slog.Logger) *exec.Cmd {
	if !cfg.AgentSDK.Enabled || cfg.AgentSDK.EvolutionDir == "" {
		return nil
	}

	port := cfg.AgentSDK.WebPort
	if port == 0 {
		port = 9998
	}

	// SDK directory: evolution_dir is sdk/agent/evolution, so sdk/ is two levels up.
	sdkDir := filepath.Dir(filepath.Dir(cfg.AgentSDK.EvolutionDir))

	// Determine python binary: prefer explicit config, then uv, then python3.
	pythonBin := cfg.AgentSDK.PythonBin
	if pythonBin == "" {
		if uvPath, err := exec.LookPath("uv"); err == nil {
			pythonBin = uvPath
		} else if py3, err := exec.LookPath("python3"); err == nil {
			pythonBin = py3
		} else {
			log.Error("cannot find python3 or uv to start agent web server")
			return nil
		}
	}

	// Build command arguments.
	var args []string
	if strings.HasSuffix(filepath.Base(pythonBin), "uv") {
		args = []string{"run", "python", "-m", "agent.web"}
	} else {
		args = []string{"-m", "agent.web"}
	}

	// Resolve mnemonic binary and config paths relative to project root.
	projectRoot := filepath.Dir(sdkDir)
	args = append(args,
		"--port", fmt.Sprintf("%d", port),
		"--mnemonic-config", filepath.Join(projectRoot, "config.yaml"),
		"--mnemonic-binary", filepath.Join(projectRoot, "bin", "mnemonic"),
	)

	cmd := exec.Command(pythonBin, args...)
	cmd.Dir = sdkDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Strip CLAUDECODE env var so the bundled Claude CLI doesn't refuse
	// to start (nested session detection).
	env := os.Environ()
	filtered := env[:0]
	for _, e := range env {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			filtered = append(filtered, e)
		}
	}
	cmd.Env = filtered

	if err := cmd.Start(); err != nil {
		log.Error("failed to start agent web server", "error", err, "python_bin", pythonBin)
		return nil
	}

	log.Info("agent web server started", "pid", cmd.Process.Pid, "port", port, "sdk_dir", sdkDir)
	return cmd
}

// serveCommand runs the mnemonic daemon.
func serveCommand(configPath string) {
	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		File:   cfg.Logging.File,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}
	slog.SetDefault(log)

	// Create data directory if it doesn't exist
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Error("failed to get home directory", "error", err)
		os.Exit(1)
	}
	dataPath := filepath.Join(homeDir, ".mnemonic")
	if err := os.MkdirAll(dataPath, 0755); err != nil {
		log.Error("failed to create data directory", "path", dataPath, "error", err)
		os.Exit(1)
	}

	// Open SQLite store
	memStore, err := sqlite.NewSQLiteStore(cfg.Store.DBPath)
	if err != nil {
		log.Error("failed to open store", "path", cfg.Store.DBPath, "error", err)
		os.Exit(1)
	}

	// Create LLM provider
	llmProvider := llm.NewLMStudioProvider(
		cfg.LLM.Endpoint,
		cfg.LLM.ChatModel,
		cfg.LLM.EmbeddingModel,
		time.Duration(cfg.LLM.TimeoutSec)*time.Second,
		cfg.LLM.MaxConcurrent,
	)

	// Create event bus
	bus := events.NewInMemoryBus(bufferSize)
	defer bus.Close()

	// Check LLM health (log warning if unavailable, don't fail startup)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.LLM.TimeoutSec)*time.Second)
	if err := llmProvider.Health(ctx); err != nil {
		log.Warn("LLM provider unavailable at startup", "endpoint", cfg.LLM.Endpoint, "error", err)
	}
	cancel()

	// Log startup info
	log.Info("mnemonic daemon starting",
		"version", Version,
		"config_path", configPath,
		"db_path", cfg.Store.DBPath,
		"llm_endpoint", cfg.LLM.Endpoint,
		"llm_chat_model", cfg.LLM.ChatModel,
		"llm_embedding_model", cfg.LLM.EmbeddingModel,
	)

	// Create a root context for all agents
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// --- Start episoding agent (groups raw events into episodes) ---
	var episodingAgent *episoding.EpisodingAgent
	if cfg.Episoding.Enabled {
		episodingCfg := episoding.EpisodingConfig{
			EpisodeWindowSizeMin: cfg.Episoding.EpisodeWindowSizeMin,
			MinEventsPerEpisode:  cfg.Episoding.MinEventsPerEpisode,
			PollingInterval:      10 * time.Second,
		}
		episodingAgent = episoding.NewEpisodingAgent(memStore, llmProvider, log, episodingCfg)
		if err := episodingAgent.Start(rootCtx, bus); err != nil {
			log.Error("failed to start episoding agent", "error", err)
		} else {
			log.Info("episoding agent started")
		}
	}

	// --- Start encoding agent ---
	var encoder *encoding.EncodingAgent
	if cfg.Encoding.Enabled {
		encoder = encoding.NewEncodingAgentWithConfig(memStore, llmProvider, log, encoding.EncodingConfig{
			PollingInterval:         5 * time.Second,
			SimilarityThreshold:     0.3,
			MaxSimilarSearchResults: cfg.Encoding.FindSimilarLimit,
			CompletionMaxTokens:     cfg.Encoding.CompletionMaxTokens,
			CompletionTemperature:   float32(cfg.LLM.Temperature),
			MaxConcurrentEncodings:  cfg.Encoding.MaxConcurrentEncodings,
			EnableLLMClassification: cfg.Encoding.EnableLLMClassification,
			CoachingFile:            cfg.Coaching.CoachingFile,
			ExcludePatterns:         cfg.Perception.Filesystem.ExcludePatterns,
		})
		if err := encoder.Start(rootCtx, bus); err != nil {
			log.Error("failed to start encoding agent", "error", err)
		} else {
			log.Info("encoding agent started")
		}
	}

	// --- Build watchers based on config ---
	var watchers []watcher.Watcher
	var percAgent *perception.PerceptionAgent

	if cfg.Perception.Enabled {
		if cfg.Perception.Filesystem.Enabled {
			// Auto-detect noisy app directories and merge with configured exclusions
			autoExclusions := fswatcher.DetectNoisyApps(log)
			allExclusions := cfg.Perception.Filesystem.ExcludePatterns
			for _, pattern := range autoExclusions {
				if !fswatcher.MatchesExcludePattern(pattern, allExclusions) {
					allExclusions = append(allExclusions, pattern)
				}
			}

			fsw, err := fswatcher.NewFilesystemWatcher(fswatcher.Config{
				WatchDirs:          cfg.Perception.Filesystem.WatchDirs,
				ExcludePatterns:    allExclusions,
				MaxContentBytes:    cfg.Perception.Filesystem.MaxContentBytes,
				MaxWatches:         cfg.Perception.Filesystem.MaxWatches,
				ShallowDepth:       cfg.Perception.Filesystem.ShallowDepth,
				PollIntervalSec:    cfg.Perception.Filesystem.PollIntervalSec,
				PromotionThreshold: cfg.Perception.Filesystem.PromotionThreshold,
				DemotionTimeoutMin: cfg.Perception.Filesystem.DemotionTimeoutMin,
			}, log)
			if err != nil {
				log.Error("failed to create filesystem watcher", "error", err)
			} else {
				watchers = append(watchers, fsw)
				log.Info("filesystem watcher configured", "dirs", cfg.Perception.Filesystem.WatchDirs)
			}
		}

		if cfg.Perception.Terminal.Enabled {
			tw, err := termwatcher.NewTerminalWatcher(termwatcher.Config{
				Shell:           cfg.Perception.Terminal.Shell,
				PollIntervalSec: cfg.Perception.Terminal.PollIntervalSec,
				ExcludePatterns: cfg.Perception.Terminal.ExcludePatterns,
			}, log)
			if err != nil {
				log.Error("failed to create terminal watcher", "error", err)
			} else {
				watchers = append(watchers, tw)
				log.Info("terminal watcher configured", "shell", cfg.Perception.Terminal.Shell)
			}
		}

		if cfg.Perception.Clipboard.Enabled {
			cw, err := clipwatcher.NewClipboardWatcher(clipwatcher.Config{
				PollIntervalSec: cfg.Perception.Clipboard.PollIntervalSec,
				MaxContentBytes: cfg.Perception.Clipboard.MaxContentBytes,
			}, log)
			if err != nil {
				log.Error("failed to create clipboard watcher", "error", err)
			} else {
				watchers = append(watchers, cw)
				log.Info("clipboard watcher configured")
			}
		}

		if cfg.Perception.Git.Enabled {
			gw, err := gitwatcher.NewGitWatcher(gitwatcher.Config{
				WatchDirs:       cfg.Perception.Filesystem.WatchDirs,
				PollIntervalSec: cfg.Perception.Git.PollIntervalSec,
				MaxRepoDepth:    cfg.Perception.Git.MaxRepoDepth,
			}, log)
			if err != nil {
				log.Warn("git watcher not available", "error", err)
			} else {
				watchers = append(watchers, gw)
				log.Info("git watcher configured")
			}
		}

		// --- Start perception agent ---
		if len(watchers) > 0 {
			percAgent = perception.NewPerceptionAgent(
				watchers,
				memStore,
				llmProvider,
				perception.PerceptionConfig{
					HeuristicConfig: perception.HeuristicConfig{
						MinContentLength:   cfg.Perception.Heuristics.MinContentLength,
						MaxContentLength:   cfg.Perception.Heuristics.MaxContentLength,
						FrequencyThreshold: cfg.Perception.Heuristics.FrequencyThreshold,
						FrequencyWindowMin: cfg.Perception.Heuristics.FrequencyWindowMin,
					},
					LLMGatingEnabled:      cfg.Perception.LLMGatingEnabled,
					LearnedExclusionsPath: cfg.Perception.LearnedExclusionsPath,
				},
				log,
			)
			if err := percAgent.Start(rootCtx, bus); err != nil {
				log.Error("failed to start perception agent", "error", err)
			} else {
				log.Info("perception agent started", "watchers", len(watchers))
			}
		}
	}

	// --- Create retrieval agent for API queries ---
	retriever := retrieval.NewRetrievalAgent(memStore, llmProvider, retrieval.RetrievalConfig{
		MaxHops:             cfg.Retrieval.MaxHops,
		ActivationThreshold: float32(cfg.Retrieval.ActivationThreshold),
		DecayFactor:         float32(cfg.Retrieval.DecayFactor),
		MaxResults:          cfg.Retrieval.MaxResults,
		MaxToolCalls:        cfg.Retrieval.MaxToolCalls,
		SynthesisMaxTokens:  cfg.Retrieval.SynthesisMaxTokens,
		MergeAlpha:          float32(cfg.Retrieval.MergeAlpha),
		DualHitBonus:        float32(cfg.Retrieval.DualHitBonus),
	}, log)

	// --- Start consolidation agent ---
	var consolidator *consolidation.ConsolidationAgent
	if cfg.Consolidation.Enabled {
		consolidator = consolidation.NewConsolidationAgent(memStore, llmProvider, consolidation.ConsolidationConfig{
			Interval:            cfg.Consolidation.Interval,
			DecayRate:           cfg.Consolidation.DecayRate,
			FadeThreshold:       cfg.Consolidation.FadeThreshold,
			ArchiveThreshold:    cfg.Consolidation.ArchiveThreshold,
			RetentionWindow:     cfg.Consolidation.RetentionWindow,
			MaxMemoriesPerCycle: cfg.Consolidation.MaxMemoriesPerCycle,
			MaxMergesPerCycle:   cfg.Consolidation.MaxMergesPerCycle,
			MinClusterSize:      cfg.Consolidation.MinClusterSize,
			AssocPruneThreshold: consolidation.DefaultConfig().AssocPruneThreshold,
		}, log)

		if err := consolidator.Start(rootCtx, bus); err != nil {
			log.Error("failed to start consolidation agent", "error", err)
		} else {
			log.Info("consolidation agent started", "interval", cfg.Consolidation.Interval)
		}
	}

	// --- Start metacognition agent ---
	var metaAgent *metacognition.MetacognitionAgent
	if cfg.Metacognition.Enabled {
		metaAgent = metacognition.NewMetacognitionAgent(memStore, llmProvider, metacognition.MetacognitionConfig{
			Interval: cfg.Metacognition.Interval,
		}, log)

		if err := metaAgent.Start(rootCtx, bus); err != nil {
			log.Error("failed to start metacognition agent", "error", err)
		} else {
			log.Info("metacognition agent started", "interval", cfg.Metacognition.Interval)
		}
	}

	// --- Start dreaming agent ---
	var dreamer *dreaming.DreamingAgent
	if cfg.Dreaming.Enabled {
		dreamer = dreaming.NewDreamingAgent(memStore, llmProvider, dreaming.DreamingConfig{
			Interval:               cfg.Dreaming.Interval,
			BatchSize:              cfg.Dreaming.BatchSize,
			SalienceThreshold:      cfg.Dreaming.SalienceThreshold,
			AssociationBoostFactor: cfg.Dreaming.AssociationBoostFactor,
			NoisePruneThreshold:    cfg.Dreaming.NoisePruneThreshold,
		}, log)

		if err := dreamer.Start(rootCtx, bus); err != nil {
			log.Error("failed to start dreaming agent", "error", err)
		} else {
			log.Info("dreaming agent started", "interval", cfg.Dreaming.Interval)
		}
	}

	// --- Start abstraction agent ---
	var abstractionAgent *abstraction.AbstractionAgent
	if cfg.Abstraction.Enabled {
		abstractionAgent = abstraction.NewAbstractionAgent(memStore, llmProvider, abstraction.AbstractionConfig{
			Interval:    cfg.Abstraction.Interval,
			MinStrength: cfg.Abstraction.MinStrength,
			MaxLLMCalls: cfg.Abstraction.MaxLLMCalls,
		}, log)

		if err := abstractionAgent.Start(rootCtx, bus); err != nil {
			log.Error("failed to start abstraction agent", "error", err)
		} else {
			log.Info("abstraction agent started", "interval", cfg.Abstraction.Interval)
		}
	}

	// --- Start orchestrator (autonomous health monitoring and self-testing) ---
	var orch *orchestrator.Orchestrator
	if cfg.Orchestrator.Enabled {
		orch = orchestrator.NewOrchestrator(memStore, llmProvider, orchestrator.OrchestratorConfig{
			AdaptiveIntervals: cfg.Orchestrator.AdaptiveIntervals,
			MaxDBSizeMB:       cfg.Orchestrator.MaxDBSizeMB,
			SelfTestInterval:  cfg.Orchestrator.SelfTestInterval,
			AutoRecovery:      cfg.Orchestrator.AutoRecovery,
			HealthReportPath:  filepath.Join(dataPath, "health.json"),
			MonitorInterval:   cfg.Orchestrator.MonitorInterval,
		}, log)

		if err := orch.Start(rootCtx, bus); err != nil {
			log.Error("failed to start orchestrator", "error", err)
		} else {
			log.Info("orchestrator started",
				"monitor_interval", cfg.Orchestrator.MonitorInterval,
				"self_test_interval", cfg.Orchestrator.SelfTestInterval)
		}
	}

	// --- Start reactor engine (centralized autonomous behavior coordination) ---
	{
		reactorLog := log.With("component", "reactor")
		reactorEngine := reactor.NewEngine(memStore, bus, reactorLog)

		deps := reactor.ChainDeps{
			MaxDBSizeMB: cfg.Orchestrator.MaxDBSizeMB,
			Logger:      reactorLog,
		}
		if consolidator != nil {
			deps.ConsolidationTrigger = consolidator.GetTriggerChannel()
		}
		if abstractionAgent != nil {
			deps.AbstractionTrigger = abstractionAgent.GetTriggerChannel()
		}
		if metaAgent != nil {
			deps.MetacognitionTrigger = metaAgent.GetTriggerChannel()
		}
		if dreamer != nil {
			deps.DreamingTrigger = dreamer.GetTriggerChannel()
		}
		if orch != nil {
			deps.IncrementAutonomous = orch.IncrementAutonomousCount
		}

		for _, chain := range reactor.NewChainRegistry(deps) {
			reactorEngine.RegisterChain(chain)
		}

		if err := reactorEngine.Start(rootCtx, bus); err != nil {
			log.Error("failed to start reactor engine", "error", err)
		}
	}

	// --- Start API server ---
	if cfg.API.Port > 0 {
		apiDeps := api.ServerDeps{
			Store:                 memStore,
			LLM:                   llmProvider,
			Bus:                   bus,
			Retriever:             retriever,
			IngestExcludePatterns: cfg.Perception.Filesystem.ExcludePatterns,
			IngestMaxContentBytes: cfg.Perception.Filesystem.MaxContentBytes,
			Log:                   log,
		}
		// Only set Consolidator if it's non-nil (avoids Go nil-interface trap)
		if consolidator != nil {
			apiDeps.Consolidator = consolidator
		}
		if cfg.AgentSDK.Enabled && cfg.AgentSDK.EvolutionDir != "" {
			apiDeps.AgentEvolutionDir = cfg.AgentSDK.EvolutionDir
			apiDeps.AgentWebPort = cfg.AgentSDK.WebPort
		}

		apiServer := api.NewServer(api.ServerConfig{
			Host:              cfg.API.Host,
			Port:              cfg.API.Port,
			RequestTimeoutSec: cfg.API.RequestTimeoutSec,
		}, apiDeps)

		if err := apiServer.Start(); err != nil {
			log.Error("failed to start API server", "error", err)
		} else {
			log.Info("API server started", "addr", fmt.Sprintf("%s:%d", cfg.API.Host, cfg.API.Port))
			defer func() {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				_ = apiServer.Stop(shutdownCtx)
			}()
		}
	}

	// --- Start agent web server (Python WebSocket) ---
	agentWebCmd := startAgentWebServer(cfg, log)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Block until signal received
	sig := <-sigChan
	log.Info("shutdown signal received", "signal", sig.String())

	// Graceful shutdown: cancel root context to stop all agents
	rootCancel()

	// Stop agent web server if running
	if agentWebCmd != nil && agentWebCmd.Process != nil {
		log.Info("stopping agent web server", "pid", agentWebCmd.Process.Pid)
		if err := agentWebCmd.Process.Signal(syscall.SIGTERM); err != nil {
			log.Warn("failed to send SIGTERM to agent web server", "error", err)
			_ = agentWebCmd.Process.Kill()
		}
		done := make(chan error, 1)
		go func() { done <- agentWebCmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			log.Warn("agent web server did not exit in 5s, killing")
			_ = agentWebCmd.Process.Kill()
		}
	}

	// Give agents a moment to drain
	time.Sleep(500 * time.Millisecond)

	if orch != nil {
		_ = orch.Stop()
	}
	if abstractionAgent != nil {
		_ = abstractionAgent.Stop()
	}
	if dreamer != nil {
		_ = dreamer.Stop()
	}
	if metaAgent != nil {
		_ = metaAgent.Stop()
	}
	if consolidator != nil {
		_ = consolidator.Stop()
	}
	if encoder != nil {
		_ = encoder.Stop()
	}
	if episodingAgent != nil {
		_ = episodingAgent.Stop()
	}
	if percAgent != nil {
		_ = percAgent.Stop()
	}

	if err := bus.Close(); err != nil {
		log.Error("error closing event bus", "error", err)
	}

	if err := memStore.Close(); err != nil {
		log.Error("error closing store", "error", err)
	}

	log.Info("mnemonic daemon shutdown complete")
}

// ============================================================================
// CLI Commands (remember / recall / consolidate)
// ============================================================================

// initRuntime loads config, opens store and LLM for CLI commands.
func initRuntime(configPath string) (*config.Config, *sqlite.SQLiteStore, *llm.LMStudioProvider, *slog.Logger) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{Level: "warn", Format: "text"})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing logger: %v\n", err)
		os.Exit(1)
	}

	homeDir, _ := os.UserHomeDir()
	dataPath := filepath.Join(homeDir, ".mnemonic")
	_ = os.MkdirAll(dataPath, 0755)

	db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening store: %v\n", err)
		os.Exit(1)
	}

	llmProvider := llm.NewLMStudioProvider(
		cfg.LLM.Endpoint,
		cfg.LLM.ChatModel,
		cfg.LLM.EmbeddingModel,
		time.Duration(cfg.LLM.TimeoutSec)*time.Second,
		cfg.LLM.MaxConcurrent,
	)

	return cfg, db, llmProvider, log
}

// rememberCommand stores text in the memory system.
// If the daemon is running, it writes the raw memory to the DB and notifies the
// daemon via API so the daemon's own encoding agent picks it up (no duplicate encoder).
// If the daemon is NOT running, it spins up a local encoder and waits for it to finish.
func rememberCommand(configPath, text string) {
	cfg, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	// Write raw memory
	raw := store.RawMemory{
		ID:              uuid.New().String(),
		Timestamp:       time.Now(),
		Source:          "user",
		Type:            "explicit",
		Content:         text,
		InitialSalience: 0.7,
		CreatedAt:       time.Now(),
	}
	if err := db.WriteRaw(ctx, raw); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing raw memory: %v\n", err)
		os.Exit(1)
	}

	// If daemon is running, just write raw and let the daemon's encoder handle it.
	// The daemon's encoding agent polls for unprocessed raw memories every 5s.
	if running, _ := daemon.IsRunning(); running {
		fmt.Printf("Remembered: %s\n", text)
		fmt.Printf("  (daemon is running — encoding will happen automatically)\n")
		return
	}

	// Daemon not running — spin up a local encoder with a generous timeout
	fmt.Printf("Encoding locally (daemon not running)...\n")

	timeoutSec := cfg.LLM.TimeoutSec
	if timeoutSec < 60 {
		timeoutSec = 60
	}
	encodeCtx, encodeCancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer encodeCancel()

	bus := events.NewInMemoryBus(100)
	defer bus.Close()

	encoder := encoding.NewEncodingAgentWithConfig(db, llmProvider, log, encoding.EncodingConfig{
		PollingInterval:         5 * time.Second,
		SimilarityThreshold:     0.3,
		MaxSimilarSearchResults: cfg.Encoding.FindSimilarLimit,
		CompletionMaxTokens:     cfg.Encoding.CompletionMaxTokens,
		CompletionTemperature:   float32(cfg.LLM.Temperature),
		MaxConcurrentEncodings:  cfg.Encoding.MaxConcurrentEncodings,
		EnableLLMClassification: cfg.Encoding.EnableLLMClassification,
		CoachingFile:            cfg.Coaching.CoachingFile,
		ExcludePatterns:         cfg.Perception.Filesystem.ExcludePatterns,
	})
	if err := encoder.Start(encodeCtx, bus); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting encoder: %v\n", err)
		os.Exit(1)
	}

	// Publish event to trigger encoding
	_ = bus.Publish(encodeCtx, events.RawMemoryCreated{
		ID:       raw.ID,
		Source:   raw.Source,
		Salience: raw.InitialSalience,
		Ts:       raw.Timestamp,
	})

	// Poll until the raw memory is marked processed or we time out
	deadline := time.After(time.Duration(timeoutSec) * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	encoded := false
	for !encoded {
		select {
		case <-deadline:
			fmt.Fprintf(os.Stderr, "Warning: encoding timed out after %ds\n", timeoutSec)
			encoded = true
		case <-ticker.C:
			r, err := db.GetRaw(ctx, raw.ID)
			if err == nil && r.Processed {
				encoded = true
			}
		}
	}

	_ = encoder.Stop()
	fmt.Printf("Remembered: %s\n", text)
}

// recallCommand retrieves memories matching a query.
func recallCommand(configPath, query string) {
	cfg, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	retriever := retrieval.NewRetrievalAgent(db, llmProvider, retrieval.RetrievalConfig{
		MaxHops:             cfg.Retrieval.MaxHops,
		ActivationThreshold: float32(cfg.Retrieval.ActivationThreshold),
		DecayFactor:         float32(cfg.Retrieval.DecayFactor),
		MaxResults:          cfg.Retrieval.MaxResults,
		MaxToolCalls:        cfg.Retrieval.MaxToolCalls,
		SynthesisMaxTokens:  cfg.Retrieval.SynthesisMaxTokens,
		MergeAlpha:          float32(cfg.Retrieval.MergeAlpha),
		DualHitBonus:        float32(cfg.Retrieval.DualHitBonus),
	}, log)

	resp, err := retriever.Query(ctx, retrieval.QueryRequest{
		Query:      query,
		Synthesize: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recalling: %v\n", err)
		os.Exit(1)
	}

	if len(resp.Memories) == 0 {
		fmt.Println("No memories found.")
		return
	}

	fmt.Printf("Found %d memories (took %dms):\n\n", len(resp.Memories), resp.TookMs)
	for i, result := range resp.Memories {
		fmt.Printf("  %d. [%.2f] %s\n", i+1, result.Score, result.Memory.Summary)
		if result.Memory.Content != "" && result.Memory.Content != result.Memory.Summary {
			fmt.Printf("     %s\n", result.Memory.Content)
		}
		fmt.Println()
	}

	if resp.Synthesis != "" {
		fmt.Printf("Synthesis:\n  %s\n", resp.Synthesis)
	}
}

// consolidateCommand runs a single memory consolidation cycle.
func consolidateCommand(configPath string) {
	cfg, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer bus.Close()

	consolidator := consolidation.NewConsolidationAgent(db, llmProvider, consolidation.ConsolidationConfig{
		Interval:            cfg.Consolidation.Interval,
		DecayRate:           cfg.Consolidation.DecayRate,
		FadeThreshold:       cfg.Consolidation.FadeThreshold,
		ArchiveThreshold:    cfg.Consolidation.ArchiveThreshold,
		RetentionWindow:     cfg.Consolidation.RetentionWindow,
		MaxMemoriesPerCycle: cfg.Consolidation.MaxMemoriesPerCycle,
		MaxMergesPerCycle:   cfg.Consolidation.MaxMergesPerCycle,
		MinClusterSize:      cfg.Consolidation.MinClusterSize,
		AssocPruneThreshold: consolidation.DefaultConfig().AssocPruneThreshold,
	}, log)

	fmt.Println("Running consolidation cycle...")

	report, err := consolidator.RunOnce(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Consolidation failed: %v\n", err)
		os.Exit(1)
	}

	// Publish events for dashboard
	_ = bus.Publish(ctx, events.ConsolidationCompleted{
		DurationMs:         report.Duration.Milliseconds(),
		MemoriesProcessed:  report.MemoriesProcessed,
		MemoriesDecayed:    report.MemoriesDecayed,
		MergedClusters:     report.MergesPerformed,
		AssociationsPruned: report.AssociationsPruned,
		Ts:                 time.Now(),
	})

	fmt.Printf("Consolidation complete (%dms):\n", report.Duration.Milliseconds())
	fmt.Printf("  Memories processed:  %d\n", report.MemoriesProcessed)
	fmt.Printf("  Salience decayed:    %d\n", report.MemoriesDecayed)
	fmt.Printf("  Transitioned fading: %d\n", report.TransitionedFading)
	fmt.Printf("  Transitioned archived: %d\n", report.TransitionedArchived)
	fmt.Printf("  Associations pruned: %d\n", report.AssociationsPruned)
	fmt.Printf("  Merges performed:    %d\n", report.MergesPerformed)
	fmt.Printf("  Expired deleted:     %d\n", report.ExpiredDeleted)
}

// ============================================================================
// Export / Import / Backup Commands
// ============================================================================

// exportCommand exports the memory store to a file.
func exportCommand(configPath string, args []string) {
	cfg, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	// Parse flags
	format := "json"
	outputPath := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "--output":
			if i+1 < len(args) {
				outputPath = args[i+1]
				i++
			}
		}
	}

	// Default output path
	if outputPath == "" {
		backupDir, err := backup.EnsureBackupDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating backup directory: %v\n", err)
			os.Exit(1)
		}
		timestamp := time.Now().Format("2006-01-02_150405")
		outputPath = filepath.Join(backupDir, fmt.Sprintf("export_%s.%s", timestamp, format))
	}

	switch format {
	case "json":
		fmt.Printf("Exporting to JSON: %s\n", outputPath)
		if err := backup.ExportJSON(ctx, db, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
			os.Exit(1)
		}
	case "sqlite":
		fmt.Printf("Exporting SQLite copy: %s\n", outputPath)
		if err := backup.ExportSQLite(ctx, cfg.Store.DBPath, outputPath); err != nil {
			fmt.Fprintf(os.Stderr, "Export failed: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Unknown format: %s (supported: json, sqlite)\n", format)
		os.Exit(1)
	}

	// Get file size
	if info, err := os.Stat(outputPath); err == nil {
		fmt.Printf("%sExport complete.%s (%.1f KB)\n", colorGreen, colorReset, float64(info.Size())/1024)
	} else {
		fmt.Printf("%sExport complete.%s\n", colorGreen, colorReset)
	}
}

// importCommand imports memories from a JSON export file.
func importCommand(configPath, filePath string, args []string) {
	_, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	// Parse mode
	mode := backup.ModeMerge
	for i := 2; i < len(args); i++ {
		if args[i] == "--mode" && i+1 < len(args) {
			switch args[i+1] {
			case "merge":
				mode = backup.ModeMerge
			case "replace":
				mode = backup.ModeReplace
			default:
				fmt.Fprintf(os.Stderr, "Unknown mode: %s (supported: merge, replace)\n", args[i+1])
				os.Exit(1)
			}
			i++
		}
	}

	fmt.Printf("Importing from %s (mode: %s)...\n", filePath, mode)

	result, err := backup.ImportFromJSON(ctx, db, filePath, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Import failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sImport complete%s (%dms):\n", colorGreen, colorReset, result.Duration.Milliseconds())
	fmt.Printf("  Memories imported:     %d\n", result.MemoriesImported)
	fmt.Printf("  Associations imported: %d\n", result.AssociationsImported)
	fmt.Printf("  Raw memories imported: %d\n", result.RawMemoriesImported)
	fmt.Printf("  Skipped duplicates:    %d\n", result.SkippedDuplicates)
	if len(result.Errors) > 0 {
		fmt.Printf("  %sWarnings:%s %d\n", colorYellow, colorReset, len(result.Errors))
	}
}

// backupCommand creates a timestamped backup with retention.
func backupCommand(configPath string) {
	_, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	backupDir, err := backup.EnsureBackupDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating backup directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Backing up to %s...\n", backupDir)

	backupPath, err := backup.BackupWithRetention(ctx, db, backupDir, 5)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Backup failed: %v\n", err)
		os.Exit(1)
	}

	if info, err := os.Stat(backupPath); err == nil {
		fmt.Printf("%sBackup complete.%s %s (%.1f KB)\n", colorGreen, colorReset, filepath.Base(backupPath), float64(info.Size())/1024)
	} else {
		fmt.Printf("%sBackup complete.%s %s\n", colorGreen, colorReset, filepath.Base(backupPath))
	}
}

// ============================================================================
// Purge Command (reset database)
// ============================================================================

// purgeCommand stops the daemon, deletes the database and log, and starts fresh.
func purgeCommand(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Confirm with user
	fmt.Printf("%sThis will permanently delete all memories and reset the database.%s\n", colorRed, colorReset)
	fmt.Printf("  Database: %s\n", cfg.Store.DBPath)
	fmt.Printf("\nType 'yes' to confirm: ")

	var confirmation string
	_, _ = fmt.Scanln(&confirmation)
	if confirmation != "yes" {
		fmt.Println("Aborted.")
		return
	}

	// Stop daemon if running
	if running, pid := daemon.IsRunning(); running {
		fmt.Printf("Stopping daemon (PID %d)...\n", pid)
		if err := daemon.Stop(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to stop daemon: %v\n", err)
			fmt.Fprintf(os.Stderr, "Please stop it manually and try again.\n")
			os.Exit(1)
		}
		time.Sleep(1 * time.Second)
	}

	// Resolve DB path (handle ~ expansion)
	dbPath := cfg.Store.DBPath
	if strings.HasPrefix(dbPath, "~") {
		home, _ := os.UserHomeDir()
		dbPath = filepath.Join(home, dbPath[1:])
	}

	// Delete database file and WAL/SHM files
	deleted := 0
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := dbPath + suffix
		if _, err := os.Stat(path); err == nil {
			if err := os.Remove(path); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to delete %s: %v\n", path, err)
			} else {
				deleted++
			}
		}
	}

	if deleted > 0 {
		fmt.Printf("%sDatabase purged.%s Deleted %d file(s).\n", colorGreen, colorReset, deleted)
	} else {
		fmt.Printf("No database files found at %s (already clean).\n", dbPath)
	}

	fmt.Println("\nThe database will be recreated automatically on next start.")
	fmt.Printf("  mnemonic start\n")
}

// ============================================================================
// Cleanup Command (selective noise removal)
// ============================================================================

// cleanupCommand scans raw_memories for paths matching exclude patterns and
// bulk-marks them as processed, then archives any encoded memories derived from them.
func cleanupCommand(configPath string, args []string) {
	cfg, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	patterns := cfg.Perception.Filesystem.ExcludePatterns
	if len(patterns) == 0 {
		fmt.Println("No exclude patterns configured in config.yaml — nothing to clean.")
		return
	}

	// Check for --yes flag
	autoConfirm := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			autoConfirm = true
		}
	}

	// Count what would be cleaned
	rawCount, err := db.CountRawUnprocessedByPathPatterns(ctx, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error counting raw memories: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sCleanup Summary%s\n", colorBold, colorReset)
	fmt.Printf("  Exclude patterns:       %d (from config.yaml)\n", len(patterns))
	fmt.Printf("  Unprocessed raw events:  %s%d%s matching exclude patterns\n", colorYellow, rawCount, colorReset)

	if rawCount == 0 {
		fmt.Println("\nNothing to clean up.")
		return
	}

	if !autoConfirm {
		fmt.Printf("\nThis will mark matching raw events as processed and archive derived memories.\n")
		fmt.Printf("Type 'yes' to confirm: ")
		var confirmation string
		_, _ = fmt.Scanln(&confirmation)
		if confirmation != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	// Mark raw events as processed
	rawCleaned, err := db.BulkMarkRawProcessedByPathPatterns(ctx, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cleaning raw memories: %v\n", err)
		os.Exit(1)
	}

	// Archive derived encoded memories
	memArchived, err := db.ArchiveMemoriesByRawPathPatterns(ctx, patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error archiving memories: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%sCleanup complete%s\n", colorGreen, colorReset)
	fmt.Printf("  Raw events marked processed:  %d\n", rawCleaned)
	fmt.Printf("  Encoded memories archived:    %d\n", memArchived)
}

// ============================================================================
// Insights Command (metacognition)
// ============================================================================

// insightsCommand displays recent metacognition observations.
func insightsCommand(configPath string) {
	_, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	observations, err := db.ListMetaObservations(ctx, "", 20)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error fetching insights: %v\n", err)
		os.Exit(1)
	}

	if len(observations) == 0 {
		fmt.Println("No insights available yet. The metacognition agent runs periodically to analyze memory health.")
		fmt.Println("Run manually with: mnemonic meta-cycle")
		return
	}

	fmt.Printf("%sMnemonic Insights%s\n\n", colorBold, colorReset)

	for _, obs := range observations {
		// Severity color
		severityColor := colorGray
		switch obs.Severity {
		case "warning":
			severityColor = colorYellow
		case "critical":
			severityColor = colorRed
		case "info":
			severityColor = colorCyan
		}

		// Format observation type
		typeLabel := strings.ReplaceAll(obs.ObservationType, "_", " ")
		typeLabel = strings.ToUpper(typeLabel[:1]) + typeLabel[1:]

		ago := time.Since(obs.CreatedAt).Round(time.Minute)
		timeStr := formatDuration(ago)
		if timeStr != "just now" {
			timeStr += " ago"
		}
		fmt.Printf("  %s[%s]%s %s%s%s (%s)\n",
			severityColor, strings.ToUpper(obs.Severity), colorReset,
			colorBold, typeLabel, colorReset,
			timeStr)

		// Print details
		for key, val := range obs.Details {
			keyLabel := strings.ReplaceAll(key, "_", " ")
			fmt.Printf("    %s: %s\n", keyLabel, formatDetailValue(val))
		}
		fmt.Println()
	}
}

// formatDetailValue renders a detail value in a human-friendly way.
func formatDetailValue(val interface{}) string {
	switch v := val.(type) {
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%.1f%%", v*100)
	case map[string]interface{}:
		parts := []string{}
		for k, mv := range v {
			switch n := mv.(type) {
			case float64:
				parts = append(parts, fmt.Sprintf("%s=%d", k, int64(n)))
			default:
				parts = append(parts, fmt.Sprintf("%s=%v", k, mv))
			}
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// metaCycleCommand runs a single metacognition cycle and displays results.
func metaCycleCommand(configPath string) {
	_, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer bus.Close()

	agent := metacognition.NewMetacognitionAgent(db, llmProvider, metacognition.MetacognitionConfig{
		Interval: 24 * time.Hour, // doesn't matter for RunOnce
	}, log)

	fmt.Println("Running metacognition cycle...")

	report, err := agent.RunOnce(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Metacognition cycle failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sMetacognition complete%s (%dms):\n", colorGreen, colorReset, report.Duration.Milliseconds())

	if len(report.Observations) == 0 {
		fmt.Println("  No issues found — memory health looks good.")
		return
	}

	fmt.Printf("  %d observation(s):\n\n", len(report.Observations))
	for _, obs := range report.Observations {
		severityColor := colorGray
		switch obs.Severity {
		case "warning":
			severityColor = colorYellow
		case "critical":
			severityColor = colorRed
		case "info":
			severityColor = colorCyan
		}

		typeLabel := strings.ReplaceAll(obs.ObservationType, "_", " ")
		typeLabel = strings.ToUpper(typeLabel[:1]) + typeLabel[1:]

		fmt.Printf("  %s[%s]%s %s\n", severityColor, strings.ToUpper(obs.Severity), colorReset, typeLabel)
		for key, val := range obs.Details {
			keyLabel := strings.ReplaceAll(key, "_", " ")
			fmt.Printf("    %s: %v\n", keyLabel, val)
		}
		fmt.Println()
	}
}

// dreamCycleCommand runs a single dream cycle and displays results.
func dreamCycleCommand(configPath string) {
	cfg, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer bus.Close()

	agent := dreaming.NewDreamingAgent(db, llmProvider, dreaming.DreamingConfig{
		Interval:               3 * time.Hour, // doesn't matter for RunOnce
		BatchSize:              cfg.Dreaming.BatchSize,
		SalienceThreshold:      cfg.Dreaming.SalienceThreshold,
		AssociationBoostFactor: cfg.Dreaming.AssociationBoostFactor,
		NoisePruneThreshold:    cfg.Dreaming.NoisePruneThreshold,
	}, log)

	fmt.Println("Running dream cycle (memory replay)...")

	report, err := agent.RunOnce(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Dream cycle failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%sDream cycle complete%s (%dms):\n", colorGreen, colorReset, report.Duration.Milliseconds())
	fmt.Printf("  Memories replayed:         %d\n", report.MemoriesReplayed)
	fmt.Printf("  Associations strengthened: %d\n", report.AssociationsStrengthened)
	fmt.Printf("  New associations created:  %d\n", report.NewAssociationsCreated)
	fmt.Printf("  Noisy memories demoted:    %d\n", report.NoisyMemoriesDemoted)
}

// mcpCommand runs the MCP server on stdin/stdout for AI agent integration.
func mcpCommand(configPath string) {
	cfg, db, llmProvider, log := initRuntime(configPath)
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewInMemoryBus(100)
	defer bus.Close()

	// Create encoding agent so remembered memories get encoded
	encoder := encoding.NewEncodingAgentWithConfig(db, llmProvider, log, encoding.EncodingConfig{
		PollingInterval:         5 * time.Second,
		SimilarityThreshold:     0.3,
		MaxSimilarSearchResults: cfg.Encoding.FindSimilarLimit,
		CompletionMaxTokens:     cfg.Encoding.CompletionMaxTokens,
		CompletionTemperature:   float32(cfg.LLM.Temperature),
		MaxConcurrentEncodings:  cfg.Encoding.MaxConcurrentEncodings,
		EnableLLMClassification: cfg.Encoding.EnableLLMClassification,
		CoachingFile:            cfg.Coaching.CoachingFile,
		ExcludePatterns:         cfg.Perception.Filesystem.ExcludePatterns,
	})
	if err := encoder.Start(ctx, bus); err != nil {
		log.Error("failed to start encoding agent for MCP", "error", err)
	}
	defer func() { _ = encoder.Stop() }()

	// Create retrieval agent for recall
	retriever := retrieval.NewRetrievalAgent(db, llmProvider, retrieval.RetrievalConfig{
		MaxHops:             cfg.Retrieval.MaxHops,
		ActivationThreshold: float32(cfg.Retrieval.ActivationThreshold),
		DecayFactor:         float32(cfg.Retrieval.DecayFactor),
		MaxResults:          cfg.Retrieval.MaxResults,
		MaxToolCalls:        cfg.Retrieval.MaxToolCalls,
		SynthesisMaxTokens:  cfg.Retrieval.SynthesisMaxTokens,
		MergeAlpha:          float32(cfg.Retrieval.MergeAlpha),
		DualHitBonus:        float32(cfg.Retrieval.DualHitBonus),
	}, log)

	server := mcp.NewMCPServer(db, retriever, bus, log, Version, cfg.Coaching.CoachingFile, cfg.Perception.Filesystem.ExcludePatterns, cfg.Perception.Filesystem.MaxContentBytes)

	// Handle signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

// ============================================================================
// Usage
// ============================================================================

// printUsage prints the command usage.
func printUsage() {
	usage := `mnemonic v%s - A semantic memory system daemon

USAGE:
  mnemonic [OPTIONS] [COMMAND]

OPTIONS:
  --config PATH    Path to config.yaml (default: "config.yaml")
  --help          Show this help message

DAEMON COMMANDS:
  start           Start the mnemonic daemon (background)
  stop            Stop the running daemon
  restart         Restart the daemon
  serve           Run in foreground (for debugging)

MEMORY COMMANDS:
  remember TEXT   Store text in memory
  recall QUERY    Retrieve memories matching query
  consolidate     Run memory consolidation cycle

DATA MANAGEMENT:
  ingest DIR      Bulk-ingest a directory (--dry-run, --project NAME)
  export          Export memories (--format json|sqlite, --output path)
  import FILE     Import from JSON export (--mode merge|replace)
  backup          Timestamped backup with retention (keeps last 5)
  cleanup         Remove noise: mark excluded-path raw events as processed (--yes)
  purge           Stop daemon and delete all data (fresh start)
  insights        Show metacognition observations (memory health)
  meta-cycle      Run a single metacognition analysis cycle
  dream-cycle     Run a single dream replay cycle

AI AGENT INTEGRATION:
  mcp             Run MCP server on stdin/stdout (for AI agents)

MONITORING COMMANDS:
  status          Show comprehensive system status
  watch           Live stream of daemon events

SETUP COMMANDS:
  install         Install as system service (auto-start on login)
  uninstall       Remove system service
  version         Show version

EXAMPLES:
  mnemonic start                                    Start daemon
  mnemonic status                                   Check everything
  mnemonic watch                                    Live event stream
  mnemonic remember "I learned something today"     Store a memory
  mnemonic recall "important lessons"               Retrieve memories
  mnemonic ingest ~/Projects/myapp --project myapp   Ingest a project
  mnemonic export --format json                     Export all data
  mnemonic backup                                   Quick backup
  mnemonic insights                                 Memory health report
  mnemonic dream-cycle                              Run dream replay
  mnemonic mcp                                      Start MCP server (stdio)
  mnemonic install                                  Auto-start on boot
  mnemonic autopilot                                Autonomous activity log
`
	fmt.Printf(usage, Version)
}

// autopilotCommand shows what the system has been doing autonomously.
func autopilotCommand(configPath string) {
	_, db, _, _ := initRuntime(configPath)
	defer db.Close()

	ctx := context.Background()

	// Read health report
	homeDir, _ := os.UserHomeDir()
	healthPath := filepath.Join(homeDir, ".mnemonic", "health.json")
	data, err := os.ReadFile(healthPath)

	fmt.Println("=== Mnemonic Autopilot Report ===")
	fmt.Println()

	if err == nil {
		var report orchestrator.HealthReport
		if json.Unmarshal(data, &report) == nil {
			fmt.Printf("Last report:     %s\n", report.Timestamp.Format("2006-01-02 15:04:05"))
			fmt.Printf("Uptime:          %s\n", report.Uptime)
			fmt.Printf("LLM available:   %v\n", report.LLMAvailable)
			fmt.Printf("Store healthy:   %v\n", report.StoreHealthy)
			fmt.Printf("Memories:        %d\n", report.MemoryCount)
			fmt.Printf("Patterns:        %d\n", report.PatternCount)
			fmt.Printf("Abstractions:    %d\n", report.AbstractionCount)
			fmt.Printf("Last consolidation: %s\n", report.LastConsolidation)
			fmt.Printf("Autonomous actions: %d\n", report.AutonomousActions)

			if len(report.Warnings) > 0 {
				fmt.Println()
				fmt.Println("Warnings:")
				for _, w := range report.Warnings {
					fmt.Printf("  - %s\n", w)
				}
			}
		}
	} else {
		fmt.Println("No health report found. Start the daemon to generate one.")
	}

	// Show recent autonomous actions
	fmt.Println()
	fmt.Println("--- Recent Autonomous Actions ---")
	actions, err := db.ListMetaObservations(ctx, "autonomous_action", 10)
	if err == nil && len(actions) > 0 {
		for _, a := range actions {
			action := ""
			if act, ok := a.Details["action"].(string); ok {
				action = act
			}
			fmt.Printf("  [%s] %s (severity: %s)\n",
				a.CreatedAt.Format("2006-01-02 15:04"), action, a.Severity)
		}
	} else {
		fmt.Println("  No autonomous actions recorded yet.")
	}

	// Show recent patterns discovered
	fmt.Println()
	fmt.Println("--- Discovered Patterns ---")
	patterns, err := db.ListPatterns(ctx, "", 5)
	if err == nil && len(patterns) > 0 {
		for _, p := range patterns {
			project := ""
			if p.Project != "" {
				project = fmt.Sprintf(" [%s]", p.Project)
			}
			fmt.Printf("  %s%s: %s (strength: %.2f, evidence: %d)\n",
				p.Title, project, p.Description, p.Strength, len(p.EvidenceIDs))
		}
	} else {
		fmt.Println("  No patterns discovered yet.")
	}

	// Show abstractions
	fmt.Println()
	fmt.Println("--- Abstractions ---")
	hasAbstractions := false
	for _, level := range []int{2, 3} {
		abs, err := db.ListAbstractions(ctx, level, 5)
		if err == nil && len(abs) > 0 {
			hasAbstractions = true
			for _, a := range abs {
				levelLabel := "principle"
				if a.Level == 3 {
					levelLabel = "axiom"
				}
				fmt.Printf("  [%s] %s: %s (confidence: %.2f)\n",
					levelLabel, a.Title, a.Description, a.Confidence)
			}
		}
	}
	if !hasAbstractions {
		fmt.Println("  No abstractions generated yet.")
	}

	fmt.Println()
}
