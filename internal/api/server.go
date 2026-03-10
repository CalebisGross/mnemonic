package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/appsprout/mnemonic/internal/agent/retrieval"
	"github.com/appsprout/mnemonic/internal/api/routes"
	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/llm"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/appsprout/mnemonic/internal/web"
)

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host              string
	Port              int
	RequestTimeoutSec int
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
	s.mux.HandleFunc("GET /api/v1/health", routes.HandleHealth(s.deps.Store, s.deps.LLM, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/stats", routes.HandleStats(s.deps.Store, s.deps.Log))

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
	s.mux.HandleFunc("GET /api/v1/abstractions", routes.HandleListAbstractions(s.deps.Store, s.deps.Log))
	s.mux.HandleFunc("GET /api/v1/projects", routes.HandleListProjects(s.deps.Store, s.deps.Log))

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

// allowedCORSOrigins is the set of origins allowed for CORS requests.
var allowedCORSOrigins = map[string]bool{
	"http://localhost:3000": true,
	"http://localhost:8080": true,
	"http://127.0.0.1:3000": true,
	"http://127.0.0.1:8080": true,
	"http://localhost:9999": true,
	"http://127.0.0.1:9999": true,
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
		if allowedCORSOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
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
