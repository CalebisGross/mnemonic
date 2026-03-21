package api

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/api/routes"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/llm"
	"github.com/appsprout-dev/mnemonic/internal/store"
	"github.com/appsprout-dev/mnemonic/internal/web"
)

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host              string
	Port              int
	RequestTimeoutSec int
	Token             string // bearer token for API auth (empty = no auth)
}

// ServerDeps holds dependencies injected into the server.
type ServerDeps struct {
	Store                 store.Store
	LLM                   llm.Provider
	Bus                   events.Bus
	Retriever             *retrieval.RetrievalAgent
	Consolidator          routes.ConsolidationRunner // can be nil if disabled
	AgentEvolutionDir     string                     // empty = agent dashboard disabled
	AgentWebPort          int                        // 0 = agent chat disabled
	IngestExcludePatterns []string
	IngestMaxContentBytes int
	Version               string
	ConfigPath            string                  // config file path for PID-based restart
	ServiceRestarter      routes.ServiceRestarter // can be nil if not installed as service
	PIDRestart            routes.PIDRestartFunc   // fallback restart when service manager unavailable
	MCPToolCount          int                     // number of registered MCP tools
	Log                   *slog.Logger
}

// Server is the HTTP API server for the Mnemonic system.
type Server struct {
	config ServerConfig
	deps   ServerDeps
	mux    *http.ServeMux
	srv    *http.Server
}

// NewServer creates a new HTTP server with the given configuration and dependencies.
func NewServer(cfg ServerConfig, deps ServerDeps) *Server {
	mux := http.NewServeMux()

	s := &Server{
		config: cfg,
		deps:   deps,
		mux:    mux,
	}

	s.registerRoutes()

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	s.srv = &http.Server{
		Addr:         addr,
		Handler:      s.middleware(mux),
		ReadTimeout:  time.Duration(cfg.RequestTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.RequestTimeoutSec) * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s
}

// registerRoutes registers all API routes with the mux.
func (s *Server) registerRoutes() {
	// Health and stats
	s.mux.HandleFunc("GET /api/v1/health", routes.HandleHealth(s.deps.Store, s.deps.LLM, s.deps.Version, s.deps.MCPToolCount, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/stats", routes.HandleStats(s.deps.Store, s.deps.Log))

	// Self-update
	s.mux.HandleFunc("GET /api/v1/system/update-check", routes.HandleUpdateCheck(s.deps.Version, s.deps.Log))
	s.mux.HandleFunc("POST /api/v1/system/update", routes.HandleUpdate(s.deps.Version, s.deps.ServiceRestarter, s.deps.PIDRestart, s.deps.ConfigPath, s.deps.Log))

	// Memory CRUD
	s.mux.HandleFunc("POST /api/v1/memories", routes.HandleCreateMemory(s.deps.Store, s.deps.Bus, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/memories", routes.HandleListMemories(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/memories/{id}", routes.HandleGetMemory(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/memories/{id}/context", routes.HandleMemoryContext(s.deps.Store, s.deps.Log))

	// Raw memory access
	s.mux.HandleFunc("GET /api/v1/raw/{id}", routes.HandleGetRawMemory(s.deps.Store, s.deps.Log))

	// Episodes
	s.mux.HandleFunc("GET /api/v1/episodes", routes.HandleListEpisodes(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/episodes/{id}", routes.HandleGetEpisode(s.deps.Store, s.deps.Log))

	// Query and retrieval
	s.mux.HandleFunc("POST /api/v1/query", routes.HandleQuery(s.deps.Retriever, s.deps.Bus, s.deps.Store, s.deps.Log))

	// Activity (watcher-derived concept tracker for MCP sync)
	s.mux.HandleFunc("GET /api/v1/activity", routes.HandleActivity(s.deps.Retriever, s.deps.Log))

	// Retrieval stats
	s.mux.HandleFunc("GET /api/v1/retrieval/stats", routes.HandleRetrievalStats(s.deps.Retriever, s.deps.Log))

	// Embedding backfill
	s.mux.HandleFunc("POST /api/v1/embeddings/backfill", routes.HandleBackfillEmbeddings(s.deps.Store, s.deps.LLM, s.deps.Log))

	// Feedback
	s.mux.HandleFunc("POST /api/v1/feedback", routes.HandleFeedback(s.deps.Store, s.deps.Log))

	// Consolidation
	s.mux.HandleFunc("POST /api/v1/consolidation/run", routes.HandleConsolidationRun(s.deps.Consolidator, s.deps.Log))

	// Ingestion
	s.mux.HandleFunc("POST /api/v1/ingest", routes.HandleIngest(s.deps.Store, s.deps.Bus, s.deps.IngestExcludePatterns, s.deps.IngestMaxContentBytes, s.deps.Log))

	// Insights
	s.mux.HandleFunc("GET /api/v1/insights", routes.HandleInsights(s.deps.Store, s.deps.Log))

	// Patterns and abstractions
	s.mux.HandleFunc("GET /api/v1/patterns", routes.HandleListPatterns(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("PATCH /api/v1/patterns/{id}", routes.HandleArchivePattern(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/abstractions", routes.HandleListAbstractions(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/projects", routes.HandleListProjects(s.deps.Store, s.deps.Log))

	// LLM usage monitoring
	s.mux.HandleFunc("GET /api/v1/llm/usage", routes.HandleLLMUsage(s.deps.Store, s.deps.Log))

	// MCP tool usage analytics
	s.mux.HandleFunc("GET /api/v1/tool/usage", routes.HandleToolUsage(s.deps.Store, s.deps.Log))

	// Sessions
	s.mux.HandleFunc("GET /api/v1/sessions", routes.HandleSessions(s.deps.Store, s.deps.Log))

	// Research analytics
	s.mux.HandleFunc("GET /api/v1/analytics", routes.HandleAnalytics(s.deps.Store, s.deps.Log))

	// Graph data for D3.js visualization
	s.mux.HandleFunc("GET /api/v1/graph", routes.HandleGraph(s.deps.Store, s.deps.Log))

	// Agent SDK evolution dashboard
	if s.deps.AgentEvolutionDir != "" {
		s.mux.HandleFunc("GET /api/v1/agent/evolution", routes.HandleAgentEvolution(s.deps.AgentEvolutionDir, s.deps.Log))
		s.mux.HandleFunc("GET /api/v1/agent/changelog", routes.HandleAgentChangelog(s.deps.AgentEvolutionDir, s.deps.Log))
		s.mux.HandleFunc("GET /api/v1/agent/sessions", routes.HandleAgentSessions(s.deps.AgentEvolutionDir, s.deps.Log))
		s.mux.HandleFunc("GET /api/v1/agent/config", routes.HandleAgentConfig(s.deps.AgentWebPort, s.deps.Log))
	}

	// WebSocket
	s.mux.HandleFunc("GET /ws", routes.HandleWebSocket(s.deps.Bus, s.deps.Log))

	// Web dashboard (serve static files at root)
	if err := web.RegisterRoutes(s.mux); err != nil {
		s.deps.Log.Warn("web dashboard disabled: failed to load static files", "error", err)
	}
}

// middleware applies global middleware to the request handler.
func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Request logging middleware
		start := time.Now()
		method := r.Method
		path := r.RequestURI
		remoteAddr := r.RemoteAddr

		s.deps.Log.Debug("incoming request", "method", method, "path", path, "remote_addr", remoteAddr)

		// CORS middleware - allow localhost origins
		// Access-Control-Allow-Origin only supports a single origin value,
		// so we check the request Origin against an allowlist.
		origin := r.Header.Get("Origin")
		if routes.AllowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Bearer token auth for API routes (when token is configured)
		if s.config.Token != "" && len(r.URL.Path) > 5 && r.URL.Path[:5] == "/api/" {
			authorized := false
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				provided := auth[7:]
				authorized = subtle.ConstantTimeCompare([]byte(provided), []byte(s.config.Token)) == 1
			}
			if !authorized {
				// Also check query parameter for dashboard convenience
				qToken := r.URL.Query().Get("token")
				authorized = qToken != "" && subtle.ConstantTimeCompare([]byte(qToken), []byte(s.config.Token)) == 1
			}
			if !authorized {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}
		}

		// JSON content type for /api/ routes
		if len(r.URL.Path) > 5 && r.URL.Path[:5] == "/api/" {
			w.Header().Set("Content-Type", "application/json")
		}

		// Request timeout wrapper
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(s.config.RequestTimeoutSec)*time.Second)
		defer cancel()
		r = r.WithContext(ctx)

		// Call next handler
		next.ServeHTTP(w, r)

		// Log request completion
		duration := time.Since(start)
		s.deps.Log.Debug("request completed", "method", method, "path", path, "duration_ms", duration.Milliseconds())
	})
}

// Start begins listening for HTTP requests. Non-blocking.
func (s *Server) Start() error {
	s.deps.Log.Info("starting HTTP server", "addr", s.srv.Addr)

	go func() {
		if err := s.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.deps.Log.Error("server listen error", "error", err)
		}
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	s.deps.Log.Info("stopping HTTP server")
	return s.srv.Shutdown(ctx)
}
