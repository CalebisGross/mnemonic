package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/config"
	"github.com/appsprout-dev/mnemonic/internal/daemon"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/logger"
	"github.com/appsprout-dev/mnemonic/internal/store/sqlite"
	"github.com/appsprout-dev/mnemonic/internal/watcher"

	"github.com/appsprout-dev/mnemonic/internal/agent/abstraction"
	"github.com/appsprout-dev/mnemonic/internal/agent/consolidation"
	"github.com/appsprout-dev/mnemonic/internal/agent/dreaming"
	"github.com/appsprout-dev/mnemonic/internal/agent/encoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/episoding"
	"github.com/appsprout-dev/mnemonic/internal/agent/metacognition"
	"github.com/appsprout-dev/mnemonic/internal/agent/orchestrator"
	"github.com/appsprout-dev/mnemonic/internal/agent/perception"
	"github.com/appsprout-dev/mnemonic/internal/agent/reactor"
	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/api"
	"github.com/appsprout-dev/mnemonic/internal/backup"
	"github.com/appsprout-dev/mnemonic/internal/mcp"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/updater"

	clipwatcher "github.com/appsprout-dev/mnemonic/internal/watcher/clipboard"
	fswatcher "github.com/appsprout-dev/mnemonic/internal/watcher/filesystem"
	gitwatcher "github.com/appsprout-dev/mnemonic/internal/watcher/git"
	termwatcher "github.com/appsprout-dev/mnemonic/internal/watcher/terminal"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var Version = "dev"

const (
	defaultConfigPath = "config.yaml"
	dataDir           = "~/.mnemonic"
	bufferSize        = 1000
)

// Exit codes for structured error reporting.
const (
	exitOK         = 0
	exitGeneral    = 1  // general/unknown error
	exitConfig     = 2  // configuration error (user-fixable)
	exitDatabase   = 3  // database / data integrity error
	exitNetwork    = 4  // network / connectivity error (transient)
	exitPermission = 5  // permission / access error (user-fixable)
	exitUsage      = 64 // bad command-line usage (matches sysexits.h EX_USAGE)
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

// die prints an error message with an optional hint and exits with the given code.
func die(code int, msg string, hint string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  Try: %s\n", hint)
	}
	os.Exit(code)
}

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
			die(exitUsage, "'ingest' requires directory argument", "mnemonic ingest <directory> [--dry-run] [--project NAME]")
		}
		ingestCommand(*configPath, args[1:])
	case "remember":
		if len(args) < 2 {
			die(exitUsage, "'remember' requires text argument", "mnemonic remember \"your text here\"")
		}
		rememberCommand(*configPath, args[1])
	case "recall":
		if len(args) < 2 {
			die(exitUsage, "'recall' requires query argument", "mnemonic recall \"your query\"")
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
			die(exitUsage, "'import' requires file path argument", "mnemonic import <backup.json> [--mode merge|replace]")
		}
		importCommand(*configPath, args[1], args)
	case "backup":
		backupCommand(*configPath)
	case "restore":
		if len(args) < 2 {
			die(exitUsage, "'restore' requires a backup file path", "mnemonic restore <backup.db>")
		}
		restoreCommand(*configPath, args[1])
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
	case "diagnose":
		diagnoseCommand(*configPath)
	case "generate-token":
		generateTokenCommand()
	case "check-update":
		checkUpdateCommand()
	case "update":
		updateCommand()
	case "version":
		fmt.Printf("mnemonic v%s\n", Version)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcommand)
		printUsage()
		os.Exit(exitUsage)
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
				healthURL := fmt.Sprintf("http://%s:%d/api/v1/health", cfg.API.Host, cfg.API.Port)
				checkLLMFromAPI(healthURL, cfg.LLM.Endpoint, cfg.API.Token)
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
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	// Resolve to absolute config path (so daemon finds it after detach)
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		die(exitGeneral, fmt.Sprintf("resolving config path: %v", err), "")
	}

	// Get our binary path
	execPath, err := os.Executable()
	if err != nil {
		die(exitGeneral, fmt.Sprintf("finding executable: %v", err), "")
	}

	fmt.Printf("Starting mnemonic daemon...\n")

	pid, err := daemon.Start(execPath, absConfigPath)
	if err != nil {
		die(exitGeneral, fmt.Sprintf("starting daemon: %v", err), "mnemonic diagnose")
	}

	// Wait briefly and verify daemon is healthy via API
	time.Sleep(2 * time.Second)
	apiURL := fmt.Sprintf("http://%s:%d/api/v1/health", cfg.API.Host, cfg.API.Port)
	healthy := false
	for i := 0; i < 3; i++ {
		resp, err := apiGet(apiURL, cfg.API.Token)
		if err == nil {
			_ = resp.Body.Close()
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

		// Check if LLM is available via health endpoint
		checkLLMFromAPI(apiURL, cfg.LLM.Endpoint, cfg.API.Token)
	} else {
		fmt.Printf("%sWarning:%s Daemon started (PID %d) but health check failed.\n", colorYellow, colorReset, pid)
		fmt.Printf("  Check logs: %s\n", daemon.LogPath())
	}
}

// generateTokenCommand generates a random API token and prints it.
// ============================================================================
// Update Commands (check-update / update)
// ============================================================================

func checkUpdateCommand() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	fmt.Printf("Checking for updates...\n")
	info, err := updater.CheckForUpdate(ctx, Version)
	if err != nil {
		die(exitNetwork, "Update check failed", err.Error())
	}

	if info.UpdateAvailable {
		fmt.Printf("\n  Current:  v%s\n", info.CurrentVersion)
		fmt.Printf("  Latest:   %sv%s%s\n\n", colorGreen, info.LatestVersion, colorReset)
		fmt.Printf("  Run %smnemonic update%s to install.\n", colorBold, colorReset)
		fmt.Printf("  Release:  %s\n", info.ReleaseURL)
	} else {
		fmt.Printf("\n  %sYou're up to date!%s (v%s)\n", colorGreen, colorReset, info.CurrentVersion)
	}
}

func updateCommand() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("Checking for updates...\n")
	info, err := updater.CheckForUpdate(ctx, Version)
	if err != nil {
		die(exitNetwork, "Update check failed", err.Error())
	}

	if !info.UpdateAvailable {
		fmt.Printf("%sAlready up to date%s (v%s)\n", colorGreen, colorReset, info.CurrentVersion)
		return
	}

	fmt.Printf("Downloading v%s...\n", info.LatestVersion)
	result, err := updater.PerformUpdate(ctx, info)
	if err != nil {
		die(exitGeneral, "Update failed", err.Error())
	}

	fmt.Printf("%sUpdated: v%s → v%s%s\n", colorGreen, result.PreviousVersion, result.NewVersion, colorReset)

	// Restart daemon if it's running
	svc := daemon.NewServiceManager()
	if svc.IsInstalled() {
		running, _ := svc.IsRunning()
		if running {
			fmt.Printf("Restarting daemon...\n")
			if err := svc.Stop(); err != nil {
				fmt.Fprintf(os.Stderr, "%sWarning:%s failed to stop daemon: %v\n", colorYellow, colorReset, err)
				fmt.Printf("Restart manually: mnemonic restart\n")
				return
			}
			time.Sleep(1 * time.Second)
			if err := svc.Start(); err != nil {
				fmt.Fprintf(os.Stderr, "%sWarning:%s failed to start daemon: %v\n", colorYellow, colorReset, err)
				fmt.Printf("Start manually: mnemonic start\n")
				return
			}
			fmt.Printf("%sDaemon restarted with v%s%s\n", colorGreen, result.NewVersion, colorReset)
		}
	}
}

func generateTokenCommand() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fmt.Fprintf(os.Stderr, "Error generating token: %v\n", err)
		os.Exit(1)
	}
	token := hex.EncodeToString(b)
	fmt.Printf("Generated API token:\n\n  %s\n\n", token)
	fmt.Printf("Add this to your config.yaml:\n\n  api:\n    token: \"%s\"\n\n", token)
	fmt.Printf("Then set this environment variable for CLI tools:\n\n  export MNEMONIC_API_TOKEN=\"%s\"\n", token)
}

// apiGet performs an HTTP GET with optional bearer token auth.
func apiGet(url, token string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return http.DefaultClient.Do(req)
}

// checkLLMFromAPI queries the health endpoint and warns if LLM is unavailable.
func checkLLMFromAPI(healthURL, llmEndpoint, token string) {
	resp, err := apiGet(healthURL, token)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()

	var health map[string]interface{}
	if json.NewDecoder(resp.Body).Decode(&health) != nil {
		return
	}

	llmAvail, _ := health["llm_available"].(bool)
	if !llmAvail {
		fmt.Printf("\n  %s⚠ LLM provider is not reachable at %s%s\n", colorYellow, llmEndpoint, colorReset)
		fmt.Printf("  Memory encoding will not work until the LLM provider is running.\n")
		fmt.Printf("  Run 'mnemonic diagnose' for details.\n")
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
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	wsURL := fmt.Sprintf("ws://%s:%d/ws", cfg.API.Host, cfg.API.Port)

	fmt.Printf("%sMnemonic Live Events%s — connecting to %s\n", colorBold, colorReset, wsURL)
	fmt.Printf("Press Ctrl+C to stop.\n\n")

	// Connect to WebSocket
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		die(exitNetwork, fmt.Sprintf("connecting to daemon: %v", err), "mnemonic start")
	}
	defer func() { _ = conn.Close() }()

	// Handle Ctrl+C
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, shutdownSignals()...)

	go func() {
		<-sigChan
		fmt.Printf("\n%sStopping event watch.%s\n", colorGray, colorReset)
		_ = conn.Close()
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
	healthResp, err := apiGet(apiBase+"/health", cfg.API.Token)
	if err == nil {
		defer func() { _ = healthResp.Body.Close() }()
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
		statsResp, err := apiGet(apiBase+"/stats", cfg.API.Token)
		if err == nil {
			defer func() { _ = statsResp.Body.Close() }()
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
		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
		if err == nil {
			defer func() { _ = db.Close() }()
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

	// Encoding queue depth — direct DB query
	fmt.Printf("\n  %sEncoding Queue%s\n", colorBold, colorReset)
	{
		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
		if err == nil {
			defer func() { _ = db.Close() }()
			ctx := context.Background()
			var unprocessed int
			row := db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM raw_memories WHERE processed = 0")
			if row.Scan(&unprocessed) == nil {
				queueColor := colorGreen
				queueNote := ""
				if unprocessed > 500 {
					queueColor = colorRed
					queueNote = " (LLM may be down — run 'mnemonic diagnose')"
				} else if unprocessed > 100 {
					queueColor = colorYellow
					queueNote = " (processing)"
				}
				fmt.Printf("    Unprocessed:    %s%d%s%s\n", queueColor, unprocessed, colorReset, queueNote)
			}
		}
	}

	// Consolidation status — check last consolidation from DB
	fmt.Printf("\n  %sConsolidation%s\n", colorBold, colorReset)
	if cfg.Consolidation.Enabled {
		fmt.Printf("    Enabled:        yes (every %s)\n", cfg.Consolidation.IntervalRaw)
		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
		if err == nil {
			defer func() { _ = db.Close() }()
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
// Diagnose
// ============================================================================

// diagnoseCommand runs a series of health checks and reports PASS/FAIL/WARN.
func diagnoseCommand(configPath string) {
	fmt.Printf("%sMnemonic v%s — Diagnostics%s\n\n", colorBold, Version, colorReset)

	passed, warned, failed := 0, 0, 0

	pass := func(label, detail string) {
		fmt.Printf("  %-16s %sPASS%s  %s\n", label, colorGreen, colorReset, detail)
		passed++
	}
	warn := func(label, detail string) {
		fmt.Printf("  %-16s %sWARN%s  %s\n", label, colorYellow, colorReset, detail)
		warned++
	}
	fail := func(label, detail string) {
		fmt.Printf("  %-16s %sFAIL%s  %s\n", label, colorRed, colorReset, detail)
		failed++
	}

	// 1. Config
	cfg, err := config.Load(configPath)
	if err != nil {
		fail("Config", fmt.Sprintf("failed to load %s: %v", configPath, err))
		// Can't continue most checks without config
		fmt.Printf("\n  %s%d passed, %d warnings, %d failed%s\n\n", colorBold, passed, warned, failed, colorReset)
		if failed > 0 {
			os.Exit(1)
		}
		return
	}
	pass("Config", fmt.Sprintf("loaded from %s", configPath))

	// 2. Data directory
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		fail("Data dir", fmt.Sprintf("cannot determine home directory: %v", homeErr))
	} else {
		dataPath := filepath.Join(home, ".mnemonic")
		info, err := os.Stat(dataPath)
		if err != nil {
			warn("Data dir", fmt.Sprintf("%s does not exist (will be created on first serve)", dataPath))
		} else if !info.IsDir() {
			fail("Data dir", fmt.Sprintf("%s exists but is not a directory", dataPath))
		} else {
			// Check writable by creating a temp file
			tmpPath := filepath.Join(dataPath, ".diagnose_test")
			if err := os.WriteFile(tmpPath, []byte("test"), 0600); err != nil {
				fail("Data dir", fmt.Sprintf("%s is not writable: %v", dataPath, err))
			} else {
				_ = os.Remove(tmpPath)
				pass("Data dir", dataPath)
			}
		}
	}

	// 3. Database
	var diagDB *sqlite.SQLiteStore
	dbInfo, dbErr := os.Stat(cfg.Store.DBPath)
	if dbErr != nil {
		fail("Database", fmt.Sprintf("file not found: %s", cfg.Store.DBPath))
	} else {
		dbSizeMB := float64(dbInfo.Size()) / (1024 * 1024)

		db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
		if err != nil {
			fail("Database", fmt.Sprintf("cannot open: %v", err))
		} else {
			diagDB = db
			defer func() { _ = diagDB.Close() }()
			ctx := context.Background()

			// Integrity check
			var integrityResult string
			row := diagDB.DB().QueryRowContext(ctx, "PRAGMA integrity_check")
			if err := row.Scan(&integrityResult); err != nil {
				fail("Database", fmt.Sprintf("integrity check error: %v", err))
			} else if integrityResult != "ok" {
				fail("Database", fmt.Sprintf("integrity check: %s", integrityResult))
			} else {
				stats, err := diagDB.GetStatistics(ctx)
				if err != nil {
					warn("Database", fmt.Sprintf("integrity OK but stats failed: %v", err))
				} else {
					pass("Database", fmt.Sprintf("integrity OK, %d memories (%d active), %.1f MB",
						stats.TotalMemories, stats.ActiveMemories, dbSizeMB))
				}
			}
		}
	}

	// 4. LLM provider
	llmProvider := newLLMProvider(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := llmProvider.Health(ctx); err != nil {
		fail("LLM", fmt.Sprintf("LLM provider not reachable at %s (%v)", cfg.LLM.Endpoint, err))
	} else {
		// Try a quick embedding to verify the model works
		_, embErr := llmProvider.Embed(ctx, "test")
		if embErr != nil {
			warn("LLM", fmt.Sprintf("reachable at %s but embedding failed: %v", cfg.LLM.Endpoint, embErr))
		} else {
			pass("LLM", fmt.Sprintf("model %s at %s", cfg.LLM.ChatModel, cfg.LLM.Endpoint))
		}
	}

	// 5. Daemon
	svc := daemon.NewServiceManager()
	if svcRunning, svcPid := svc.IsRunning(); svcRunning {
		pass("Daemon", fmt.Sprintf("running (%s, PID %d)", svc.ServiceName(), svcPid))
	} else if running, pid := daemon.IsRunning(); running {
		pass("Daemon", fmt.Sprintf("running (PID %d)", pid))
	} else {
		warn("Daemon", "not running — use 'mnemonic start' or 'mnemonic serve'")
	}

	// 6. Disk space
	if homeErr == nil {
		dbDir := filepath.Dir(cfg.Store.DBPath)
		availBytes, err := diskAvailable(dbDir)
		if err == nil {
			availGB := float64(availBytes) / (1024 * 1024 * 1024)
			if availGB < 1.0 {
				fail("Disk", fmt.Sprintf("%.1f GB available on %s — critically low", availGB, dbDir))
			} else if availGB < 5.0 {
				warn("Disk", fmt.Sprintf("%.1f GB available on %s", availGB, dbDir))
			} else {
				pass("Disk", fmt.Sprintf("%.0f GB available", availGB))
			}
		}
		// If we can't check disk, just skip silently (platform-specific)
	}

	// 7. Encoding queue (reuse DB connection from check 3)
	if diagDB != nil {
		ctx := context.Background()
		var unprocessed int
		row := diagDB.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM raw_memories WHERE processed = 0")
		if row.Scan(&unprocessed) == nil {
			if unprocessed > 500 {
				warn("Encoding queue", fmt.Sprintf("%d unprocessed raw memories (LLM may be falling behind)", unprocessed))
			} else {
				pass("Encoding queue", fmt.Sprintf("%d unprocessed", unprocessed))
			}
		}
	}

	// Summary
	fmt.Println()
	if failed > 0 {
		fmt.Printf("  %s%d passed, %d warnings, %d failed%s\n\n", colorRed, passed, warned, failed, colorReset)
		os.Exit(1)
	} else if warned > 0 {
		fmt.Printf("  %s%d passed, %d warnings%s\n\n", colorYellow, passed, warned, colorReset)
	} else {
		fmt.Printf("  %sAll %d checks passed%s\n\n", colorGreen, passed, colorReset)
	}
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
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	// Resolve paths
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		die(exitGeneral, fmt.Sprintf("resolving config path: %v", err), "")
	}

	execPath, err := os.Executable()
	if err != nil {
		die(exitGeneral, fmt.Sprintf("finding executable: %v", err), "")
	}

	if err := svc.Install(execPath, absConfigPath); err != nil {
		die(exitPermission, fmt.Sprintf("installing service: %v", err), "check system permissions")
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
// Returns the started Cmd and a channel that receives the Wait() result when the
// process exits. The caller must use the channel instead of calling cmd.Wait()
// directly, since the background monitor goroutine owns the single Wait() call.
// Returns (nil, nil) if disabled or failed to start.
func startAgentWebServer(cfg *config.Config, log *slog.Logger) (*exec.Cmd, <-chan error) {
	if !cfg.AgentSDK.Enabled || cfg.AgentSDK.EvolutionDir == "" {
		return nil, nil
	}

	port := cfg.AgentSDK.WebPort
	if port == 0 {
		port = 9998
	}

	// SDK directory: evolution_dir is sdk/agent/evolution, so sdk/ is two levels up.
	sdkDir := filepath.Dir(filepath.Dir(cfg.AgentSDK.EvolutionDir))

	// Determine python binary: prefer explicit config, then venv Python (has
	// all SDK deps installed), then uv, then system python3/python.
	pythonBin := cfg.AgentSDK.PythonBin
	if pythonBin == "" {
		// Venv layout differs by platform: bin/python3 (Unix) vs Scripts/python.exe (Windows)
		venvPython := filepath.Join(sdkDir, ".venv", "bin", "python3")
		if runtime.GOOS == "windows" {
			venvPython = filepath.Join(sdkDir, ".venv", "Scripts", "python.exe")
		}
		if _, err := os.Stat(venvPython); err == nil {
			pythonBin = venvPython
		} else if uvPath, err := exec.LookPath("uv"); err == nil {
			pythonBin = uvPath
		} else if py3, err := exec.LookPath("python3"); err == nil {
			pythonBin = py3
		} else if py, err := exec.LookPath("python"); err == nil {
			// Windows typically has "python" not "python3"
			pythonBin = py
		} else {
			log.Error("cannot find python3 or uv to start agent web server")
			return nil, nil
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
	binaryName := "mnemonic"
	if runtime.GOOS == "windows" {
		binaryName = "mnemonic.exe"
	}
	args = append(args,
		"--port", fmt.Sprintf("%d", port),
		"--mnemonic-config", filepath.Join(projectRoot, "config.yaml"),
		"--mnemonic-binary", filepath.Join(projectRoot, "bin", binaryName),
	)

	cmd := exec.Command(pythonBin, args...)
	cmd.Dir = sdkDir

	// Capture stderr so missing-dependency tracebacks don't pollute the console.
	var stderrBuf bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderrBuf

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
		return nil, nil
	}

	log.Info("agent web server started", "pid", cmd.Process.Pid, "port", port, "sdk_dir", sdkDir)

	// Monitor the process in background — if it exits quickly, log a clean warning
	// instead of dumping a raw Python traceback. This goroutine owns the single
	// cmd.Wait() call; the done channel lets the shutdown path wait for exit
	// without calling Wait() a second time (which would race).
	done := make(chan error, 1)
	go func() {
		err := cmd.Wait()
		if err != nil {
			stderr := strings.TrimSpace(stderrBuf.String())
			if strings.Contains(stderr, "ModuleNotFoundError") || strings.Contains(stderr, "No module named") {
				log.Warn("agent web server exited: missing Python dependency — install SDK requirements to enable",
					"hint", "cd sdk && pip install -r requirements.txt")
			} else {
				log.Warn("agent web server exited unexpectedly", "error", err, "stderr", stderr)
			}
		}
		done <- err
	}()

	return cmd, done
}

// serveCommand runs the mnemonic daemon.
func serveCommand(configPath string) {
	// If running as a Windows Service, delegate to the service handler.
	if daemon.IsWindowsService() {
		execPath, _ := os.Executable()
		if err := daemon.RunAsService(execPath, configPath); err != nil {
			die(exitGeneral, fmt.Sprintf("running as Windows service: %v", err), "")
		}
		return
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	// Check config file permissions
	if warn := config.WarnPermissions(configPath); warn != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", warn)
	}

	// Build project resolver from config
	projectResolver := config.NewProjectResolver(cfg.Projects)

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		File:   cfg.Logging.File,
	})
	if err != nil {
		die(exitConfig, fmt.Sprintf("initializing logger: %v", err), "check logging config in config.yaml")
	}
	slog.SetDefault(log)

	// Create data directory if it doesn't exist
	if err := cfg.EnsureDataDir(); err != nil {
		die(exitPermission, fmt.Sprintf("creating data directory: %v", err), "check permissions on ~/.mnemonic/")
	}

	// Pre-migration safety backup (only if DB already exists)
	if _, statErr := os.Stat(cfg.Store.DBPath); statErr == nil {
		backupDir, bdErr := backup.EnsureBackupDir()
		if bdErr != nil {
			log.Warn("could not create backup directory for pre-migration backup", "error", bdErr)
		} else {
			bkPath, bkErr := backup.BackupSQLiteFile(cfg.Store.DBPath, backupDir)
			if bkErr != nil {
				log.Warn("pre-migration backup failed", "error", bkErr)
			} else if bkPath != "" {
				log.Info("pre-migration backup created", "path", bkPath)
			}
		}
	}

	// Open SQLite store
	memStore, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
	if err != nil {
		die(exitDatabase, fmt.Sprintf("opening database %s: %v", cfg.Store.DBPath, err), "mnemonic diagnose")
	}

	// Run integrity check on startup
	intCtx, intCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if intErr := memStore.CheckIntegrity(intCtx); intErr != nil {
		log.Error("database integrity check failed", "error", intErr)
		fmt.Fprintf(os.Stderr, "\n%s✗ DATABASE CORRUPTION DETECTED%s\n", colorRed, colorReset)
		fmt.Fprintf(os.Stderr, "  %v\n", intErr)
		fmt.Fprintf(os.Stderr, "  A pre-migration backup was saved. Use 'mnemonic restore <backup>' to recover.\n\n")
	} else {
		log.Info("database integrity check passed")
	}
	intCancel()

	// Check available disk space
	dbDir := filepath.Dir(cfg.Store.DBPath)
	if availBytes, diskErr := diskAvailable(dbDir); diskErr == nil {
		availMB := availBytes / (1024 * 1024)
		if availMB < 100 {
			log.Error("critically low disk space", "available_mb", availMB, "path", dbDir)
			fmt.Fprintf(os.Stderr, "\n%s✗ CRITICALLY LOW DISK SPACE: %d MB available%s\n", colorRed, availMB, colorReset)
			fmt.Fprintf(os.Stderr, "  Database writes may fail. Free up disk space before continuing.\n\n")
		} else if availMB < 500 {
			log.Warn("low disk space", "available_mb", availMB, "path", dbDir)
			fmt.Fprintf(os.Stderr, "\n%s⚠ Low disk space: %d MB available%s\n", colorYellow, availMB, colorReset)
		}
	}

	// Create LLM provider
	llmProvider := newLLMProvider(cfg)

	// Check for embedding model drift
	embModel := cfg.LLM.EmbeddingModel
	if cfg.LLM.Provider == "embedded" && cfg.LLM.Embedded.EmbedModelFile != "" {
		embModel = cfg.LLM.Embedded.EmbedModelFile
	}
	if embModel != "" {
		metaCtx, metaCancel := context.WithTimeout(context.Background(), 5*time.Second)
		prevModel, _ := memStore.GetMeta(metaCtx, "embedding_model")
		metaCancel()

		if prevModel != "" && prevModel != embModel {
			log.Warn("embedding model changed", "previous", prevModel, "current", embModel)
			fmt.Fprintf(os.Stderr, "\n%s⚠ Embedding model changed: %s → %s%s\n", colorYellow, prevModel, embModel, colorReset)
			fmt.Fprintf(os.Stderr, "  Existing semantic search may return degraded results.\n")
			fmt.Fprintf(os.Stderr, "  Old embeddings are from a different vector space.\n\n")
		}

		metaCtx2, metaCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
		_ = memStore.SetMeta(metaCtx2, "embedding_model", embModel)
		metaCancel2()
	}

	// Create event bus
	bus := events.NewInMemoryBus(bufferSize)
	defer func() { _ = bus.Close() }()

	// Check LLM health (warn loudly if unavailable, don't fail startup)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.LLM.TimeoutSec)*time.Second)
	if err := llmProvider.Health(ctx); err != nil {
		log.Warn("LLM provider unavailable at startup", "endpoint", cfg.LLM.Endpoint, "error", err)
		fmt.Fprintf(os.Stderr, "\n%s⚠ WARNING: LLM provider is not reachable at %s%s\n", colorYellow, cfg.LLM.Endpoint, colorReset)
		fmt.Fprintf(os.Stderr, "  Memory encoding will not work until the LLM provider is running.\n")
		fmt.Fprintf(os.Stderr, "  Raw observations will queue and be processed once the LLM provider is available.\n")
		fmt.Fprintf(os.Stderr, "  Run 'mnemonic diagnose' for a full health check.\n\n")
	}
	cancel()

	// Log startup info
	embCount, embLoadTime := memStore.EmbeddingIndexStats()
	log.Info("mnemonic daemon starting",
		"version", Version,
		"config_path", configPath,
		"db_path", cfg.Store.DBPath,
		"llm_endpoint", cfg.LLM.Endpoint,
		"llm_chat_model", cfg.LLM.ChatModel,
		"llm_embedding_model", cfg.LLM.EmbeddingModel,
		"embedding_index_size", embCount,
		"embedding_index_load_ms", embLoadTime.Milliseconds(),
	)
	if embCount > 50000 {
		log.Warn("large embedding index — consider ANN index for better performance",
			"count", embCount, "load_ms", embLoadTime.Milliseconds())
	}

	// Create a root context for all agents
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	// Instrumented provider wrapper — gives each agent its own usage tracking.
	// If training data capture is enabled, wrap with TrainingCaptureProvider too.
	wrap := func(caller string) llm.Provider {
		var p llm.Provider = llm.NewInstrumentedProvider(llmProvider, memStore, caller, cfg.LLM.ChatModel)
		if cfg.Training.CaptureEnabled && cfg.Training.CaptureDir != "" {
			p = llm.NewTrainingCaptureProvider(p, caller, cfg.Training.CaptureDir)
		}
		return p
	}

	// --- Start episoding agent (groups raw events into episodes) ---
	var episodingAgent *episoding.EpisodingAgent
	if cfg.Episoding.Enabled {
		episodingCfg := episoding.EpisodingConfig{
			EpisodeWindowSizeMin: cfg.Episoding.EpisodeWindowSizeMin,
			MinEventsPerEpisode:  cfg.Episoding.MinEventsPerEpisode,
			PollingInterval:      10 * time.Second,
		}
		episodingAgent = episoding.NewEpisodingAgent(memStore, wrap("episoding"), log, episodingCfg)
		if err := episodingAgent.Start(rootCtx, bus); err != nil {
			log.Error("failed to start episoding agent", "error", err)
		} else {
			log.Info("episoding agent started")
		}
	}

	// --- Start encoding agent ---
	var encoder *encoding.EncodingAgent
	if cfg.Encoding.Enabled {
		encoder = encoding.NewEncodingAgentWithConfig(memStore, wrap("encoding"), log, buildEncodingConfig(cfg))
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
				SensitivePatterns:  cfg.Perception.Filesystem.SensitivePatterns,
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
				wrap("perception"),
				perception.PerceptionConfig{
					HeuristicConfig: perception.HeuristicConfig{
						MinContentLength:   cfg.Perception.Heuristics.MinContentLength,
						MaxContentLength:   cfg.Perception.Heuristics.MaxContentLength,
						FrequencyThreshold: cfg.Perception.Heuristics.FrequencyThreshold,
						FrequencyWindowMin: cfg.Perception.Heuristics.FrequencyWindowMin,
						PassScore:          float32(cfg.Perception.HeuristicPassScore),
						BatchEditWindowSec: cfg.Perception.BatchEditWindowSec,
						BatchEditThreshold: cfg.Perception.BatchEditThreshold,
						RecallBoostMax:     float32(cfg.Perception.RecallBoostMax),
						RecallBoostMinutes: cfg.Perception.RecallBoostWindowMin,
					},
					LLMGatingEnabled:      cfg.Perception.LLMGatingEnabled,
					LearnedExclusionsPath: cfg.Perception.LearnedExclusionsPath,
					ProjectResolver:       projectResolver,
					ContentDedupTTLSec:    cfg.Perception.ContentDedupTTLSec,
					GitOpCooldownSec:      cfg.Perception.GitOpCooldownSec,
					MaxRawContentLen:      cfg.Perception.MaxRawContentLen,
					LLMGateSnippetLen:     cfg.Perception.LLMGateSnippetLen,
					LLMGateTimeoutSec:     cfg.Perception.LLMGateTimeoutSec,
					RejectionThreshold:    cfg.Perception.RejectionThreshold,
					RejectionWindowMin:    cfg.Perception.RejectionWindowMin,
					RejectionMaxPromoted:  cfg.Perception.RejectionMaxPromoted,
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
	retriever := retrieval.NewRetrievalAgent(memStore, wrap("retrieval"), retrieval.RetrievalConfig{
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
		consolidator = consolidation.NewConsolidationAgent(memStore, wrap("consolidation"), consolidation.ConsolidationConfig{
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
		metaAgent = metacognition.NewMetacognitionAgent(memStore, wrap("metacognition"), metacognition.MetacognitionConfig{
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
		dreamer = dreaming.NewDreamingAgent(memStore, wrap("dreaming"), dreaming.DreamingConfig{
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
		abstractionAgent = abstraction.NewAbstractionAgent(memStore, wrap("abstraction"), abstraction.AbstractionConfig{
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
		orch = orchestrator.NewOrchestrator(memStore, wrap("orchestrator"), orchestrator.OrchestratorConfig{
			AdaptiveIntervals: cfg.Orchestrator.AdaptiveIntervals,
			MaxDBSizeMB:       cfg.Orchestrator.MaxDBSizeMB,
			SelfTestInterval:  cfg.Orchestrator.SelfTestInterval,
			AutoRecovery:      cfg.Orchestrator.AutoRecovery,
			HealthReportPath:  filepath.Join(filepath.Dir(cfg.Store.DBPath), "health.json"),
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
			Version:               Version,
			ConfigPath:            configPath,
			ServiceRestarter:      daemon.NewServiceManager(),
			PIDRestart:            daemon.PIDRestart,
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
			Token:             cfg.API.Token,
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
	agentWebCmd, agentWebDone := startAgentWebServer(cfg, log)

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, shutdownSignals()...)

	// Block until signal received
	sig := <-sigChan
	log.Info("shutdown signal received", "signal", sig.String())

	// Graceful shutdown: cancel root context to stop all agents
	rootCancel()

	// Stop agent web server if running. Use agentWebDone (owned by the
	// background goroutine) instead of calling cmd.Wait() a second time.
	if agentWebCmd != nil && agentWebCmd.Process != nil {
		log.Info("stopping agent web server", "pid", agentWebCmd.Process.Pid)
		// On Unix, send SIGTERM for graceful shutdown. On Windows, SIGTERM
		// is not supported — go straight to Kill().
		if runtime.GOOS != "windows" {
			if err := agentWebCmd.Process.Signal(syscall.SIGTERM); err != nil {
				log.Warn("failed to send SIGTERM to agent web server", "error", err)
				_ = agentWebCmd.Process.Kill()
			}
		} else {
			_ = agentWebCmd.Process.Kill()
		}
		select {
		case <-agentWebDone:
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
// The returned Provider includes training data capture if enabled in config.
func initRuntime(configPath string) (*config.Config, *sqlite.SQLiteStore, llm.Provider, *slog.Logger) {
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	log, err := logger.New(logger.Config{Level: "warn", Format: "text"})
	if err != nil {
		die(exitGeneral, fmt.Sprintf("initializing logger: %v", err), "")
	}

	_ = cfg.EnsureDataDir()

	db, err := sqlite.NewSQLiteStore(cfg.Store.DBPath, cfg.Store.BusyTimeoutMs)
	if err != nil {
		die(exitDatabase, fmt.Sprintf("opening database: %v", err), "mnemonic diagnose")
	}

	provider := newLLMProvider(cfg)

	// Wrap with training data capture if enabled
	if cfg.Training.CaptureEnabled && cfg.Training.CaptureDir != "" {
		provider = llm.NewTrainingCaptureProvider(provider, "cli", cfg.Training.CaptureDir)
	}

	return cfg, db, provider, log
}

// rememberCommand stores text in the memory system.
// If the daemon is running, it writes the raw memory to the DB and notifies the
// daemon via API so the daemon's own encoding agent picks it up (no duplicate encoder).
// If the daemon is NOT running, it spins up a local encoder and waits for it to finish.
func rememberCommand(configPath, text string) {
	const maxRememberBytes = 10240 // 10KB
	if len(text) > maxRememberBytes {
		fmt.Fprintf(os.Stderr, "Error: input too large (%d bytes, max %d). Pipe large content through 'mnemonic ingest' instead.\n", len(text), maxRememberBytes)
		os.Exit(1)
	}

	cfg, db, llmProvider, log := initRuntime(configPath)
	defer func() { _ = db.Close() }()

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
	defer func() { _ = bus.Close() }()

	encoder := encoding.NewEncodingAgentWithConfig(db, llmProvider, log, buildEncodingConfig(cfg))
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
	defer func() { _ = db.Close() }()

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
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer func() { _ = bus.Close() }()

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
	defer func() { _ = db.Close() }()

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
	defer func() { _ = db.Close() }()

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
	defer func() { _ = db.Close() }()

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
// Restore Command (disaster recovery)
// ============================================================================

// restoreCommand restores the database from a SQLite backup file.
func restoreCommand(configPath string, backupPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
	}

	// Verify backup file exists
	info, err := os.Stat(backupPath)
	if err != nil {
		die(exitUsage, fmt.Sprintf("backup file not found: %s", backupPath), "check the file path")
	}
	if info.IsDir() {
		die(exitUsage, fmt.Sprintf("%s is a directory, not a backup file", backupPath), "provide a .db file path")
	}

	// Verify backup integrity by opening it as a SQLite database
	fmt.Printf("Verifying backup integrity: %s\n", backupPath)
	testStore, err := sqlite.NewSQLiteStore(backupPath, 5000)
	if err != nil {
		die(exitDatabase, fmt.Sprintf("backup is not a valid SQLite database: %v", err), "")
	}
	intCtx, intCancel := context.WithTimeout(context.Background(), 30*time.Second)
	intErr := testStore.CheckIntegrity(intCtx)
	intCancel()
	_ = testStore.Close()
	if intErr != nil {
		die(exitDatabase, fmt.Sprintf("backup file is corrupted: %v", intErr), "")
	}
	fmt.Printf("  %s✓ Backup integrity verified%s\n", colorGreen, colorReset)

	// Check if daemon is running
	svc := daemon.NewServiceManager()
	if running, _ := svc.IsRunning(); running {
		die(exitGeneral, "daemon is running", "mnemonic stop")
	}

	// If current DB exists, move it aside
	dbPath := cfg.Store.DBPath
	if _, statErr := os.Stat(dbPath); statErr == nil {
		aside := dbPath + ".pre-restore"
		fmt.Printf("  Moving current database to %s\n", aside)
		if err := os.Rename(dbPath, aside); err != nil {
			die(exitPermission, fmt.Sprintf("moving current database: %v", err), "check file permissions")
		}
	}

	// Copy backup to DB path
	_ = cfg.EnsureDataDir()
	if err := backup.ExportSQLite(context.Background(), backupPath, dbPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error copying backup to database path: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n%s✓ Database restored from %s%s\n", colorGreen, filepath.Base(backupPath), colorReset)
	fmt.Printf("  Database: %s (%.1f KB)\n", dbPath, float64(info.Size())/1024)
	fmt.Printf("  Start the daemon with 'mnemonic start' or 'mnemonic serve'.\n")
}

// ============================================================================
// Purge Command (reset database)
// ============================================================================

// purgeCommand stops the daemon, deletes the database and log, and starts fresh.
func purgeCommand(configPath string) {
	cfg, err := config.Load(configPath)
	if err != nil {
		die(exitConfig, fmt.Sprintf("loading config: %v", err), "mnemonic diagnose")
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
	defer func() { _ = db.Close() }()

	ctx := context.Background()

	patterns := cfg.Perception.Filesystem.ExcludePatterns
	if len(patterns) == 0 {
		fmt.Println("No exclude patterns configured in config.yaml — nothing to clean.")
		return
	}

	// Check for flags
	autoConfirm := false
	cleanPatterns := false
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			autoConfirm = true
		}
		if a == "--patterns" {
			cleanPatterns = true
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
	if cleanPatterns {
		fmt.Printf("  --patterns flag:        will archive all active patterns and abstractions\n")
	}

	if rawCount == 0 && !cleanPatterns {
		fmt.Println("\nNothing to clean up.")
		return
	}

	if !autoConfirm {
		fmt.Printf("\nThis will mark matching raw events as processed and archive derived memories.\n")
		if cleanPatterns {
			fmt.Printf("It will also archive ALL active patterns and abstractions (they regenerate from clean data).\n")
		}
		fmt.Printf("Type 'yes' to confirm: ")
		var confirmation string
		_, _ = fmt.Scanln(&confirmation)
		if confirmation != "yes" {
			fmt.Println("Aborted.")
			return
		}
	}

	rawCleaned := 0
	memArchived := 0

	if rawCount > 0 {
		// Mark raw events as processed
		rawCleaned, err = db.BulkMarkRawProcessedByPathPatterns(ctx, patterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error cleaning raw memories: %v\n", err)
			os.Exit(1)
		}

		// Archive derived encoded memories
		memArchived, err = db.ArchiveMemoriesByRawPathPatterns(ctx, patterns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error archiving memories: %v\n", err)
			os.Exit(1)
		}
	}

	patternsArchived := 0
	abstractionsArchived := 0
	if cleanPatterns {
		patternsArchived, err = db.ArchiveAllPatterns(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error archiving patterns: %v\n", err)
			os.Exit(1)
		}
		abstractionsArchived, err = db.ArchiveAllAbstractions(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error archiving abstractions: %v\n", err)
			os.Exit(1)
		}
	}

	fmt.Printf("\n%sCleanup complete%s\n", colorGreen, colorReset)
	fmt.Printf("  Raw events marked processed:  %d\n", rawCleaned)
	fmt.Printf("  Encoded memories archived:    %d\n", memArchived)
	if cleanPatterns {
		fmt.Printf("  Patterns archived:            %d\n", patternsArchived)
		fmt.Printf("  Abstractions archived:        %d\n", abstractionsArchived)
	}
}

// ============================================================================
// Insights Command (metacognition)
// ============================================================================

// insightsCommand displays recent metacognition observations.
func insightsCommand(configPath string) {
	_, db, _, _ := initRuntime(configPath)
	defer func() { _ = db.Close() }()

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
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer func() { _ = bus.Close() }()

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
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	bus := events.NewInMemoryBus(100)
	defer func() { _ = bus.Close() }()

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
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := events.NewInMemoryBus(100)
	defer func() { _ = bus.Close() }()

	// Create encoding agent so remembered memories get encoded
	encoder := encoding.NewEncodingAgentWithConfig(db, llmProvider, log, buildEncodingConfig(cfg))
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

	mcpResolver := config.NewProjectResolver(cfg.Projects)
	server := mcp.NewMCPServer(db, retriever, bus, log, Version, cfg.Coaching.CoachingFile, cfg.Perception.Filesystem.ExcludePatterns, cfg.Perception.Filesystem.MaxContentBytes, mcpResolver)

	// Handle signal for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, shutdownSignals()...)
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
  restore FILE    Restore database from a SQLite backup file
  cleanup         Remove noise: mark excluded-path raw events as processed (--yes)
  purge           Stop daemon and delete all data (fresh start)
  insights        Show metacognition observations (memory health)
  meta-cycle      Run a single metacognition analysis cycle
  dream-cycle     Run a single dream replay cycle

AI AGENT INTEGRATION:
  mcp             Run MCP server on stdin/stdout (for AI agents)

MONITORING COMMANDS:
  status          Show comprehensive system status
  diagnose        Run health checks (config, DB, LLM, disk)
  watch           Live stream of daemon events

UPDATE COMMANDS:
  check-update    Check if a newer version is available
  update          Download and install the latest version

SETUP COMMANDS:
  install         Install as system service (auto-start on login)
  uninstall       Remove system service
  generate-token  Generate a random API authentication token
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
  mnemonic restore ~/.mnemonic/backups/backup.db    Restore from backup

EXIT CODES:
  0     Success
  1     General error
  2     Configuration error (check config.yaml)
  3     Database error (run 'mnemonic diagnose')
  4     Network/connectivity error (transient)
  5     Permission error (check file permissions)
  64    Bad command-line usage
`
	fmt.Printf(usage, Version)
}

// autopilotCommand shows what the system has been doing autonomously.
func autopilotCommand(configPath string) {
	_, db, _, _ := initRuntime(configPath)
	defer func() { _ = db.Close() }()

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

// buildEncodingConfig translates central config into the encoding agent's config struct.
func buildEncodingConfig(cfg *config.Config) encoding.EncodingConfig {
	pollingInterval := time.Duration(cfg.Encoding.PollingIntervalSec) * time.Second
	if pollingInterval <= 0 {
		pollingInterval = 5 * time.Second
	}
	simThreshold := float32(cfg.Encoding.SimilarityThreshold)
	if simThreshold <= 0 {
		simThreshold = 0.3
	}
	return encoding.EncodingConfig{
		PollingInterval:         pollingInterval,
		SimilarityThreshold:     simThreshold,
		MaxSimilarSearchResults: cfg.Encoding.FindSimilarLimit,
		CompletionMaxTokens:     cfg.Encoding.CompletionMaxTokens,
		CompletionTemperature:   float32(cfg.LLM.Temperature),
		MaxConcurrentEncodings:  cfg.Encoding.MaxConcurrentEncodings,
		EnableLLMClassification: cfg.Encoding.EnableLLMClassification,
		CoachingFile:            cfg.Coaching.CoachingFile,
		ExcludePatterns:         cfg.Perception.Filesystem.ExcludePatterns,
		ConceptVocabulary:       cfg.Encoding.ConceptVocabulary,
		MaxRetries:              cfg.Encoding.MaxRetries,
		MaxLLMContentChars:      cfg.Encoding.MaxLLMContentChars,
		MaxEmbeddingChars:       cfg.Encoding.MaxEmbeddingChars,
		TemporalWindowMin:       cfg.Encoding.TemporalWindowMin,
		BackoffThreshold:        cfg.Encoding.BackoffThreshold,
		BackoffBaseSec:          cfg.Encoding.BackoffBaseSec,
		BackoffMaxSec:           cfg.Encoding.BackoffMaxSec,
		BatchSizeEvent:          cfg.Encoding.BatchSizeEvent,
		BatchSizePoll:           cfg.Encoding.BatchSizePoll,
	}
}

// newLLMProvider creates the appropriate LLM provider based on config.
// For "api" (default), it creates an LMStudioProvider for OpenAI-compatible APIs.
// For "embedded", it creates an EmbeddedProvider for in-process llama.cpp inference.
func newLLMProvider(cfg *config.Config) llm.Provider {
	switch cfg.LLM.Provider {
	case "embedded":
		ep := llm.NewEmbeddedProvider(llm.EmbeddedProviderConfig{
			ModelsDir:      cfg.LLM.Embedded.ModelsDir,
			ChatModelFile:  cfg.LLM.Embedded.ChatModelFile,
			EmbedModelFile: cfg.LLM.Embedded.EmbedModelFile,
			ContextSize:    cfg.LLM.Embedded.ContextSize,
			GPULayers:      cfg.LLM.Embedded.GPULayers,
			Threads:        cfg.LLM.Embedded.Threads,
			BatchSize:      cfg.LLM.Embedded.BatchSize,
			MaxTokens:      cfg.LLM.MaxTokens,
			Temperature:    float32(cfg.LLM.Temperature),
			MaxConcurrent:  cfg.LLM.MaxConcurrent,
		})
		// Note: LoadModels must be called with a backend factory before use.
		// Until llama.cpp bindings are integrated, the provider will return
		// ErrProviderUnavailable on all inference calls.
		return ep
	default: // "api" or ""
		timeout := time.Duration(cfg.LLM.TimeoutSec) * time.Second
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		return llm.NewLMStudioProvider(
			cfg.LLM.Endpoint,
			cfg.LLM.ChatModel,
			cfg.LLM.EmbeddingModel,
			cfg.LLM.APIKey,
			timeout,
			cfg.LLM.MaxConcurrent,
		)
	}
}
