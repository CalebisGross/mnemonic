package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/appsprout/mnemonic/internal/agent/retrieval"
	"github.com/appsprout/mnemonic/internal/events"
	"github.com/appsprout/mnemonic/internal/ingest"
	"github.com/appsprout/mnemonic/internal/store"
	"github.com/google/uuid"
)

// JSON-RPC 2.0 types

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPServer implements the Model Context Protocol over JSON-RPC 2.0
type MCPServer struct {
	store           store.Store
	retriever       *retrieval.RetrievalAgent
	bus             events.Bus
	log             *slog.Logger
	version         string // binary version, injected from main
	sessionID       string // auto-generated per MCP server lifetime
	project         string // auto-detected from working directory
	coachingFile    string // path for coach_local_llm writes
	excludePatterns []string
	maxContentBytes int
}

// NewMCPServer creates a new MCP server with the given dependencies.
func NewMCPServer(s store.Store, r *retrieval.RetrievalAgent, bus events.Bus, log *slog.Logger, version string, coachingFile string, excludePatterns []string, maxContentBytes int) *MCPServer {
	// Auto-detect project from working directory
	project := detectProject()

	// Generate session ID for this MCP server lifetime
	sessionID := fmt.Sprintf("mcp-%s", uuid.New().String()[:8])

	log.Info("MCP server initialized", "session_id", sessionID, "project", project)

	return &MCPServer{
		store:           s,
		retriever:       r,
		bus:             bus,
		log:             log,
		version:         version,
		sessionID:       sessionID,
		project:         project,
		coachingFile:    coachingFile,
		excludePatterns: excludePatterns,
		maxContentBytes: maxContentBytes,
	}
}

// detectProject determines the project name from the current working directory.
func detectProject() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	// Use the last path component as project name
	parts := strings.Split(wd, string(os.PathSeparator))
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// Run starts the MCP server, reading JSON-RPC requests from stdin and writing responses to stdout.
func (srv *MCPServer) Run(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		if ctx.Err() != nil {
			break
		}

		line := scanner.Bytes()

		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			srv.log.Debug("parse error", "error", err)
			if err := enc.Encode(errorResponse(nil, -32700, "Parse error")); err != nil {
				srv.log.Warn("failed to encode error response to stdout", "error", err)
			}
			continue
		}

		srv.log.Debug("received request", "method", req.Method, "id", req.ID)

		resp := srv.handleRequest(ctx, &req)

		// Skip encoding nil responses (for notifications)
		if resp != nil {
			if err := enc.Encode(resp); err != nil {
				srv.log.Warn("failed to encode response to stdout", "error", err)
			}
		}
	}

	return scanner.Err()
}

// handleRequest dispatches the request to the appropriate handler based on method.
func (srv *MCPServer) handleRequest(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return srv.handleInitialize(req)
	case "notifications/initialized":
		return nil // notifications don't send responses
	case "tools/list":
		return srv.handleToolsList(req)
	case "tools/call":
		return srv.handleToolCall(ctx, req)
	default:
		return errorResponse(req.ID, -32601, "Method not found")
	}
}

// handleInitialize returns the MCP initialization response.
func (srv *MCPServer) handleInitialize(req *jsonRPCRequest) *jsonRPCResponse {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"serverInfo": map[string]interface{}{
			"name":    "mnemonic",
			"version": srv.version,
		},
	}
	return successResponse(req.ID, result)
}

// ToolDefinition describes an MCP tool.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// handleToolsList returns the list of available tools.
func (srv *MCPServer) handleToolsList(req *jsonRPCRequest) *jsonRPCResponse {
	tools := []ToolDefinition{
		{
			Name:        "remember",
			Description: "Store a memory in the Mnemonic memory system. Memories are automatically tagged with the current project and session. Use this to record decisions, errors, insights, or anything worth remembering.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"text": map[string]interface{}{
						"type":        "string",
						"description": "The memory content to store",
					},
					"source": map[string]interface{}{
						"type":        "string",
						"description": "The source of the memory (default: mcp)",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Memory type: decision, error, insight, learning, or general (default: general)",
						"enum":        []string{"decision", "error", "insight", "learning", "general"},
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (auto-detected from working directory if omitted)",
					},
				},
				"required": []string{"text"},
			},
		},
		{
			Name:        "recall",
			Description: "Retrieve relevant memories using semantic search and spread activation. Supports project scoping, time ranges, and concept filtering. Returns synthesized results by default.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The query to search for",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of memories to return (default: 5)",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Filter by project name",
					},
					"concepts": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "Filter by specific concepts",
					},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "forget",
			Description: "Archive a memory by ID",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"memory_id": map[string]interface{}{
						"type":        "string",
						"description": "The ID of the memory to archive",
					},
				},
				"required": []string{"memory_id"},
			},
		},
		{
			Name:        "status",
			Description: "Get memory system statistics, health insights, and project breakdown",
			InputSchema: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
				"required":   []string{},
			},
		},
		{
			Name:        "recall_project",
			Description: "Retrieve project-scoped memories with an activity summary. Shows recent memories, patterns, and key decisions for a specific project.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (uses current project if omitted)",
					},
					"query": map[string]interface{}{
						"type":        "string",
						"description": "Optional search query within the project",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of memories to return (default: 10)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "recall_timeline",
			Description: "Retrieve memories in chronological order within a time range. Useful for reconstructing what happened during a specific period.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"hours_back": map[string]interface{}{
						"type":        "integer",
						"description": "How many hours back to look (default: 24)",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of memories to return (default: 20)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "session_summary",
			Description: "Summarize the current or most recent session. Shows what was worked on, decisions made, errors encountered, and insights gained.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"session_id": map[string]interface{}{
						"type":        "string",
						"description": "Session ID (uses current session if omitted)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "get_patterns",
			Description: "Retrieve discovered patterns from the memory system. Patterns are recurring themes, practices, or behaviors detected across memories.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Filter by project name",
					},
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of patterns to return (default: 10)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "get_insights",
			Description: "Return metacognition observations and abstractions. Shows what the memory system has learned about your work patterns, knowledge gaps, and system health.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Maximum number of insights to return (default: 10)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "feedback",
			Description: "Report the quality of a recall result. Include the query_id from the recall response to enable association strength tuning. Helps the memory system learn which memories and associations are useful.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The original recall query",
					},
					"quality": map[string]interface{}{
						"type":        "string",
						"description": "Quality rating: helpful, partial, or irrelevant",
						"enum":        []string{"helpful", "partial", "irrelevant"},
					},
					"memory_ids": map[string]interface{}{
						"type":        "array",
						"items":       map[string]interface{}{"type": "string"},
						"description": "IDs of the memories that were returned",
					},
					"query_id": map[string]interface{}{
						"type":        "string",
						"description": "The query_id returned by the recall tool — enables association strength tuning based on feedback",
					},
				},
				"required": []string{"query", "quality"},
			},
		},
		{
			Name:        "audit_encodings",
			Description: "Return recent raw→encoded memory pairs for quality review. Shows what the local LLM produced from each raw observation. Use this to spot weak summaries, miscalibrated salience, or concept extraction gaps.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"limit": map[string]interface{}{
						"type":        "integer",
						"description": "Number of raw→encoded pairs to return (default: 5, max: 20)",
					},
					"hours_back": map[string]interface{}{
						"type":        "integer",
						"description": "How many hours back to look for raw memories (default: 24)",
					},
					"source": map[string]interface{}{
						"type":        "string",
						"description": "Filter by source: filesystem, terminal, clipboard, mcp (optional)",
					},
				},
				"required": []string{},
			},
		},
		{
			Name:        "coach_local_llm",
			Description: "Write coaching instructions for the local LLM's encoding agent. Writes YAML that improves how the local model encodes memories. Changes take effect after daemon restart.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"coaching_yaml": map[string]interface{}{
						"type":        "string",
						"description": "Full YAML content for the coaching file. Must have a top-level 'coaching' key with an 'encoding' sub-key containing 'notes' and 'instructions'.",
					},
				},
				"required": []string{"coaching_yaml"},
			},
		},
		{
			Name:        "ingest_project",
			Description: "Ingest a local directory into the memory system. Walks the directory, filters binary/excluded files, deduplicates against existing memories, and writes raw memories for encoding. Re-running on the same directory is safe — duplicates are skipped.",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"directory": map[string]interface{}{
						"type":        "string",
						"description": "Absolute path to the directory to ingest",
					},
					"project": map[string]interface{}{
						"type":        "string",
						"description": "Project name (default: directory basename)",
					},
					"dry_run": map[string]interface{}{
						"type":        "boolean",
						"description": "If true, scan and report without writing (default: false)",
					},
				},
				"required": []string{"directory"},
			},
		},
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	return successResponse(req.ID, result)
}

// toolCallParams represents the parameters for a tools/call request.
type toolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// handleToolCall dispatches tool calls to their respective handlers.
func (srv *MCPServer) handleToolCall(ctx context.Context, req *jsonRPCRequest) *jsonRPCResponse {
	var params toolCallParams
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return errorResponse(req.ID, -32602, "Invalid params")
	}

	var result interface{}
	var toolErr error

	switch params.Name {
	case "remember":
		result, toolErr = srv.handleRemember(ctx, params.Arguments)
	case "recall":
		result, toolErr = srv.handleRecall(ctx, params.Arguments)
	case "forget":
		result, toolErr = srv.handleForget(ctx, params.Arguments)
	case "status":
		result, toolErr = srv.handleStatus(ctx, params.Arguments)
	case "recall_project":
		result, toolErr = srv.handleRecallProject(ctx, params.Arguments)
	case "recall_timeline":
		result, toolErr = srv.handleRecallTimeline(ctx, params.Arguments)
	case "session_summary":
		result, toolErr = srv.handleSessionSummary(ctx, params.Arguments)
	case "get_patterns":
		result, toolErr = srv.handleGetPatterns(ctx, params.Arguments)
	case "get_insights":
		result, toolErr = srv.handleGetInsights(ctx, params.Arguments)
	case "feedback":
		result, toolErr = srv.handleFeedback(ctx, params.Arguments)
	case "audit_encodings":
		result, toolErr = srv.handleAuditEncodings(ctx, params.Arguments)
	case "coach_local_llm":
		result, toolErr = srv.handleCoachLocalLLM(ctx, params.Arguments)
	case "ingest_project":
		result, toolErr = srv.handleIngestProject(ctx, params.Arguments)
	default:
		return errorResponse(req.ID, -32602, fmt.Sprintf("Unknown tool: %s", params.Name))
	}

	if toolErr != nil {
		return successResponse(req.ID, toolError(toolErr.Error()))
	}

	return successResponse(req.ID, result)
}

// handleRemember stores a new memory in the system.
func (srv *MCPServer) handleRemember(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	text, ok := args["text"].(string)
	if !ok || text == "" {
		return nil, fmt.Errorf("text parameter is required and must be a string")
	}

	source := "mcp"
	if s, ok := args["source"].(string); ok {
		source = s
	}

	memType := "general"
	if t, ok := args["type"].(string); ok && t != "" {
		memType = t
	}

	project := srv.project
	if p, ok := args["project"].(string); ok && p != "" {
		project = p
	}

	raw := store.RawMemory{
		ID:              uuid.New().String(),
		Source:          source,
		Type:            memType,
		Content:         text,
		Timestamp:       time.Now(),
		CreatedAt:       time.Now(),
		HeuristicScore:  0.5,
		InitialSalience: 0.7,
		Processed:       false,
		Project:         project,
		SessionID:       srv.sessionID,
		Metadata: map[string]interface{}{
			"mcp_session_id": srv.sessionID,
			"memory_type":    memType,
			"project":        project,
		},
	}

	// Boost salience for specific types
	switch memType {
	case "decision":
		raw.InitialSalience = 0.85
	case "error":
		raw.InitialSalience = 0.8
	case "insight":
		raw.InitialSalience = 0.9
	case "learning":
		raw.InitialSalience = 0.8
	}

	if err := srv.store.WriteRaw(ctx, raw); err != nil {
		srv.log.Error("failed to write raw memory", "error", err)
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	// Publish event so encoding agent picks it up
	if err := srv.bus.Publish(ctx, events.RawMemoryCreated{
		ID:     raw.ID,
		Source: raw.Source,
		Ts:     time.Now(),
	}); err != nil {
		srv.log.Warn("failed to publish raw memory created event", "error", err)
	}

	srv.log.Info("memory stored", "id", raw.ID, "source", source, "type", memType, "project", project)

	return toolResult(fmt.Sprintf("Stored memory %s (type: %s, project: %s)", raw.ID, memType, project)), nil
}

// handleRecall retrieves memories using semantic search and spread activation.
func (srv *MCPServer) handleRecall(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a string")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Check for project-scoped recall
	project := ""
	if p, ok := args["project"].(string); ok {
		project = p
	}

	// Check for concept-filtered recall
	var concepts []string
	if c, ok := args["concepts"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				concepts = append(concepts, s)
			}
		}
	}

	// If project is specified, use project-scoped search
	if project != "" {
		memories, err := srv.store.SearchByProject(ctx, project, query, limit)
		if err != nil {
			srv.log.Error("project recall failed", "query", query, "project", project, "error", err)
			return nil, fmt.Errorf("project recall failed: %w", err)
		}
		text := fmt.Sprintf("Found %d memories in project '%s':\n\n", len(memories), project)
		for i, mem := range memories {
			text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n   Project: %s\n\n",
				i+1, mem.ID, mem.Summary, mem.Concepts, mem.Project)
		}
		return toolResult(text), nil
	}

	// If concepts are specified, use concept-based search
	if len(concepts) > 0 {
		memories, err := srv.store.SearchByConcepts(ctx, concepts, limit)
		if err != nil {
			srv.log.Error("concept recall failed", "concepts", concepts, "error", err)
			return nil, fmt.Errorf("concept recall failed: %w", err)
		}
		text := fmt.Sprintf("Found %d memories matching concepts %v:\n\n", len(memories), concepts)
		for i, mem := range memories {
			text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n\n",
				i+1, mem.ID, mem.Summary, mem.Concepts)
		}
		return toolResult(text), nil
	}

	// Default: full semantic search with spread activation
	queryReq := retrieval.QueryRequest{
		Query:               query,
		MaxResults:          limit,
		IncludeReasoning:    true,
		Synthesize:          true,
		IncludePatterns:     true,
		IncludeAbstractions: true,
	}

	result, err := srv.retriever.Query(ctx, queryReq)
	if err != nil {
		srv.log.Error("retrieval failed", "query", query, "error", err)
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// Save traversal data for feedback loop
	var retrievedIDs []string
	for _, mem := range result.Memories {
		retrievedIDs = append(retrievedIDs, mem.Memory.ID)
	}
	fb := store.RetrievalFeedback{
		QueryID:         result.QueryID,
		QueryText:       query,
		RetrievedIDs:    retrievedIDs,
		TraversedAssocs: result.TraversedAssocs,
		CreatedAt:       time.Now(),
	}
	if err := srv.store.WriteRetrievalFeedback(ctx, fb); err != nil {
		srv.log.Warn("failed to save retrieval feedback record", "query_id", result.QueryID, "error", err)
	}

	text := fmt.Sprintf("Found %d memories (query_id: %s):\n\n", len(result.Memories), result.QueryID)
	for i, mem := range result.Memories {
		projectInfo := ""
		if mem.Memory.Project != "" {
			projectInfo = fmt.Sprintf("\n   Project: %s", mem.Memory.Project)
		}
		contentSnippet := ""
		if mem.Memory.Content != "" && mem.Memory.Content != mem.Memory.Summary {
			contentSnippet = fmt.Sprintf("\n   Content: %s", mem.Memory.Content)
		}
		text += fmt.Sprintf("%d. [%.3f] %s\n   Summary: %s%s\n   Concepts: %v\n   Created: %s%s\n\n",
			i+1, mem.Score, mem.Memory.ID, mem.Memory.Summary, contentSnippet,
			mem.Memory.Concepts, mem.Memory.CreatedAt.Format("2006-01-02 15:04"), projectInfo)
	}

	if result.Synthesis != "" {
		text += fmt.Sprintf("Synthesis:\n%s\n", result.Synthesis)
	}

	if len(result.Patterns) > 0 {
		text += fmt.Sprintf("\nRelevant Patterns (%d):\n", len(result.Patterns))
		for _, p := range result.Patterns {
			text += fmt.Sprintf("  - [strength:%.2f] %s (%s): %s\n", p.Strength, p.Title, p.PatternType, p.Description)
		}
	}

	if len(result.Abstractions) > 0 {
		text += "\nApplicable Principles:\n"
		for _, a := range result.Abstractions {
			levelLabel := "principle"
			if a.Level == 3 {
				levelLabel = "axiom"
			}
			text += fmt.Sprintf("  - [%s, confidence:%.2f] %s: %s\n", levelLabel, a.Confidence, a.Title, a.Description)
		}
	}

	srv.log.Info("recall completed", "query", query, "query_id", result.QueryID, "results", len(result.Memories), "patterns", len(result.Patterns), "abstractions", len(result.Abstractions), "took_ms", result.TookMs)

	return toolResult(text), nil
}

// handleForget archives a memory by ID.
func (srv *MCPServer) handleForget(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	memoryID, ok := args["memory_id"].(string)
	if !ok || memoryID == "" {
		return nil, fmt.Errorf("memory_id parameter is required and must be a string")
	}

	if err := srv.store.UpdateState(ctx, memoryID, "archived"); err != nil {
		srv.log.Error("failed to archive memory", "id", memoryID, "error", err)
		return nil, fmt.Errorf("failed to archive memory: %w", err)
	}

	srv.log.Info("memory archived", "id", memoryID)

	return toolResult(fmt.Sprintf("Memory %s archived", memoryID)), nil
}

// handleStatus returns system statistics and health information.
func (srv *MCPServer) handleStatus(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	stats, err := srv.store.GetStatistics(ctx)
	if err != nil {
		srv.log.Error("failed to get statistics", "error", err)
		return nil, fmt.Errorf("failed to get statistics: %w", err)
	}

	observations, err := srv.store.ListMetaObservations(ctx, "", 5)
	if err != nil {
		srv.log.Warn("failed to get meta observations", "error", err)
		observations = []store.MetaObservation{}
	}

	text := "Mnemonic Status:\n\n"
	text += fmt.Sprintf("Session: %s\n", srv.sessionID)
	text += fmt.Sprintf("Project: %s\n\n", srv.project)
	text += fmt.Sprintf("Total memories: %d\n", stats.TotalMemories)
	text += fmt.Sprintf("Active: %d, Fading: %d, Archived: %d, Merged: %d\n",
		stats.ActiveMemories, stats.FadingMemories, stats.ArchivedMemories, stats.MergedMemories)
	text += fmt.Sprintf("Total associations: %d (avg %.2f per memory)\n",
		stats.TotalAssociations, stats.AvgAssociationsPerMem)
	text += fmt.Sprintf("Storage size: %.2f MB\n", float64(stats.StorageSizeBytes)/(1024*1024))

	if !stats.LastConsolidation.IsZero() {
		text += fmt.Sprintf("Last consolidation: %s\n", stats.LastConsolidation.Format("2006-01-02 15:04:05"))
	}

	// Project breakdown
	projects, err := srv.store.ListProjects(ctx)
	if err == nil && len(projects) > 0 {
		text += fmt.Sprintf("\nProjects (%d):\n", len(projects))
		for _, p := range projects {
			text += fmt.Sprintf("  - %s\n", p)
		}
	}

	if len(observations) > 0 {
		text += fmt.Sprintf("\nRecent observations (%d):\n", len(observations))
		for _, obs := range observations {
			text += fmt.Sprintf("  - [%s] %s: %s\n", obs.Severity, obs.ObservationType, obs.CreatedAt.Format("2006-01-02 15:04:05"))
		}
	}

	srv.log.Info("status retrieved", "total_memories", stats.TotalMemories)

	return toolResult(text), nil
}

// handleRecallProject retrieves project-scoped memories with an activity summary.
func (srv *MCPServer) handleRecallProject(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	project := srv.project
	if p, ok := args["project"].(string); ok && p != "" {
		project = p
	}
	if project == "" {
		return nil, fmt.Errorf("project name is required (set project param or run from a project directory)")
	}

	query := ""
	if q, ok := args["query"].(string); ok {
		query = q
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Get project summary
	summary, err := srv.store.GetProjectSummary(ctx, project)
	if err != nil {
		srv.log.Warn("failed to get project summary", "project", project, "error", err)
	}

	// Get project memories
	memories, err := srv.store.SearchByProject(ctx, project, query, limit)
	if err != nil {
		srv.log.Error("project recall failed", "project", project, "error", err)
		return nil, fmt.Errorf("project recall failed: %w", err)
	}

	// Get patterns for this project
	patterns, err := srv.store.ListPatterns(ctx, project, 5)
	if err != nil {
		srv.log.Warn("failed to get project patterns", "project", project, "error", err)
	}

	text := fmt.Sprintf("Project: %s\n\n", project)

	if summary != nil {
		if total, ok := summary["total_memories"]; ok {
			text += fmt.Sprintf("Total memories: %v\n", total)
		}
		if lastActivity, ok := summary["last_activity"]; ok {
			text += fmt.Sprintf("Last activity: %v\n", lastActivity)
		}
	}

	if len(patterns) > 0 {
		text += fmt.Sprintf("\nPatterns (%d):\n", len(patterns))
		for _, p := range patterns {
			text += fmt.Sprintf("  - [%.2f] %s: %s\n", p.Strength, p.Title, p.Description)
		}
	}

	text += fmt.Sprintf("\nMemories (%d):\n\n", len(memories))
	for i, mem := range memories {
		text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n   State: %s\n\n",
			i+1, mem.ID, mem.Summary, mem.Concepts, mem.State)
	}

	srv.log.Info("project recall completed", "project", project, "memories", len(memories))

	return toolResult(text), nil
}

// handleRecallTimeline retrieves memories in chronological order within a time range.
func (srv *MCPServer) handleRecallTimeline(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	hoursBack := 24
	if h, ok := args["hours_back"].(float64); ok {
		hoursBack = int(h)
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	from := time.Now().Add(-time.Duration(hoursBack) * time.Hour)
	to := time.Now()

	memories, err := srv.store.ListMemoriesByTimeRange(ctx, from, to, limit)
	if err != nil {
		srv.log.Error("timeline recall failed", "error", err)
		return nil, fmt.Errorf("timeline recall failed: %w", err)
	}

	text := fmt.Sprintf("Timeline (last %dh, %d memories):\n\n", hoursBack, len(memories))
	for i, mem := range memories {
		projectInfo := ""
		if mem.Project != "" {
			projectInfo = fmt.Sprintf(" [%s]", mem.Project)
		}
		text += fmt.Sprintf("%d. %s%s\n   %s\n   Concepts: %v\n\n",
			i+1, mem.Timestamp.Format("2006-01-02 15:04:05"), projectInfo,
			mem.Summary, mem.Concepts)
	}

	srv.log.Info("timeline recall completed", "hours_back", hoursBack, "memories", len(memories))

	return toolResult(text), nil
}

// handleSessionSummary summarizes the current or specified session.
func (srv *MCPServer) handleSessionSummary(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID := srv.sessionID
	if s, ok := args["session_id"].(string); ok && s != "" {
		sessionID = s
	}

	// Get the open episode for session context
	episode, err := srv.store.GetOpenEpisode(ctx)
	if err != nil {
		srv.log.Debug("no open episode for session summary", "error", err)
	}

	// Get recent memories from this session (by time, since session_id on memories
	// is set during encoding, we look at recent activity)
	from := time.Now().Add(-12 * time.Hour)
	memories, err := srv.store.ListMemoriesByTimeRange(ctx, from, time.Now(), 20)
	if err != nil {
		srv.log.Error("failed to get session memories", "error", err)
		return nil, fmt.Errorf("failed to get session memories: %w", err)
	}

	text := fmt.Sprintf("Session Summary: %s\n", sessionID)
	text += fmt.Sprintf("Project: %s\n\n", srv.project)

	if episode.ID != "" {
		text += fmt.Sprintf("Current episode: %s (%d events)\n", episode.ID, len(episode.RawMemoryIDs))
		if episode.Summary != "" {
			text += fmt.Sprintf("Episode summary: %s\n", episode.Summary)
		}
		text += "\n"
	}

	if len(memories) == 0 {
		text += "No memories recorded in this session yet.\n"
	} else {
		// Categorize memories by type
		decisions := 0
		errors := 0
		insights := 0
		for _, mem := range memories {
			for _, c := range mem.Concepts {
				switch {
				case strings.Contains(c, "decision"):
					decisions++
				case strings.Contains(c, "error"):
					errors++
				case strings.Contains(c, "insight"):
					insights++
				}
			}
		}

		text += fmt.Sprintf("Activity: %d memories", len(memories))
		if decisions > 0 {
			text += fmt.Sprintf(", %d decisions", decisions)
		}
		if errors > 0 {
			text += fmt.Sprintf(", %d errors", errors)
		}
		if insights > 0 {
			text += fmt.Sprintf(", %d insights", insights)
		}
		text += "\n\nRecent:\n"

		showCount := len(memories)
		if showCount > 10 {
			showCount = 10
		}
		for i := 0; i < showCount; i++ {
			mem := memories[i]
			text += fmt.Sprintf("  %d. [%s] %s\n", i+1, mem.Timestamp.Format("15:04"), mem.Summary)
		}
	}

	srv.log.Info("session summary generated", "session_id", sessionID, "memories", len(memories))

	return toolResult(text), nil
}

// handleGetPatterns retrieves discovered patterns.
func (srv *MCPServer) handleGetPatterns(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	project := ""
	if p, ok := args["project"].(string); ok {
		project = p
	}

	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	patterns, err := srv.store.ListPatterns(ctx, project, limit)
	if err != nil {
		srv.log.Error("failed to list patterns", "error", err)
		return nil, fmt.Errorf("failed to list patterns: %w", err)
	}

	if len(patterns) == 0 {
		return toolResult("No patterns discovered yet. Patterns emerge as the system processes more memories and runs consolidation cycles."), nil
	}

	text := fmt.Sprintf("Discovered Patterns (%d):\n\n", len(patterns))
	for i, p := range patterns {
		projectInfo := ""
		if p.Project != "" {
			projectInfo = fmt.Sprintf(" [%s]", p.Project)
		}
		text += fmt.Sprintf("%d. %s%s\n   Type: %s | Strength: %.2f | Evidence: %d memories\n   %s\n   Concepts: %v\n\n",
			i+1, p.Title, projectInfo, p.PatternType, p.Strength, len(p.EvidenceIDs),
			p.Description, p.Concepts)
	}

	srv.log.Info("patterns retrieved", "count", len(patterns))

	return toolResult(text), nil
}

// handleGetInsights returns metacognition observations and abstractions.
func (srv *MCPServer) handleGetInsights(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	observations, err := srv.store.ListMetaObservations(ctx, "", limit)
	if err != nil {
		srv.log.Error("failed to list observations", "error", err)
		return nil, fmt.Errorf("failed to list observations: %w", err)
	}

	abstractions, err := srv.store.ListAbstractions(ctx, 0, limit)
	if err != nil {
		srv.log.Warn("failed to list abstractions", "error", err)
	}

	text := "Mnemonic Insights:\n\n"

	if len(abstractions) > 0 {
		text += fmt.Sprintf("Abstractions (%d):\n", len(abstractions))
		for _, a := range abstractions {
			levelName := "pattern"
			switch a.Level {
			case 2:
				levelName = "principle"
			case 3:
				levelName = "axiom"
			}
			text += fmt.Sprintf("  - [L%d %s] %s (confidence: %.2f)\n    %s\n",
				a.Level, levelName, a.Title, a.Confidence, a.Description)
		}
		text += "\n"
	}

	if len(observations) > 0 {
		text += fmt.Sprintf("Observations (%d):\n", len(observations))
		for _, obs := range observations {
			text += fmt.Sprintf("  - [%s] %s (%s)\n",
				obs.Severity, obs.ObservationType, obs.CreatedAt.Format("2006-01-02 15:04"))
		}
	}

	if len(abstractions) == 0 && len(observations) == 0 {
		text += "No insights available yet. Insights emerge as the system processes more memories and runs analysis cycles.\n"
	}

	srv.log.Info("insights retrieved", "observations", len(observations), "abstractions", len(abstractions))

	return toolResult(text), nil
}

// Feedback tuning constants
const (
	feedbackStrengthDelta float32 = 0.05
	feedbackSalienceBoost float32 = 0.02
)

// handleFeedback records quality feedback for a recall result and adjusts association strengths.
func (srv *MCPServer) handleFeedback(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required")
	}

	quality, ok := args["quality"].(string)
	if !ok || quality == "" {
		return nil, fmt.Errorf("quality parameter is required (helpful, partial, or irrelevant)")
	}

	var memoryIDs []string
	if ids, ok := args["memory_ids"].([]interface{}); ok {
		for _, v := range ids {
			if s, ok := v.(string); ok {
				memoryIDs = append(memoryIDs, s)
			}
		}
	}

	queryID, _ := args["query_id"].(string)

	// Store feedback as a meta observation
	obs := store.MetaObservation{
		ID:              uuid.New().String(),
		ObservationType: "retrieval_feedback",
		Severity:        "info",
		Details: map[string]interface{}{
			"query":      query,
			"quality":    quality,
			"memory_ids": memoryIDs,
			"query_id":   queryID,
			"session_id": srv.sessionID,
			"project":    srv.project,
		},
		CreatedAt: time.Now(),
	}

	if err := srv.store.WriteMetaObservation(ctx, obs); err != nil {
		srv.log.Error("failed to write feedback", "error", err)
		return nil, fmt.Errorf("failed to store feedback: %w", err)
	}

	// If query_id is provided, look up traversal data and adjust association strengths
	adjustments := 0
	if queryID != "" {
		fb, err := srv.store.GetRetrievalFeedback(ctx, queryID)
		if err != nil {
			srv.log.Warn("failed to look up retrieval feedback record", "query_id", queryID, "error", err)
		} else {
			// Update the feedback record with the quality rating
			fb.Feedback = quality
			_ = srv.store.WriteRetrievalFeedback(ctx, fb)

			switch quality {
			case "helpful":
				// Strengthen traversed associations and boost returned memory salience
				for _, ta := range fb.TraversedAssocs {
					assocs, err := srv.store.GetAssociations(ctx, ta.SourceID)
					if err != nil {
						continue
					}
					for _, a := range assocs {
						if a.TargetID == ta.TargetID {
							newStrength := a.Strength + feedbackStrengthDelta
							if newStrength > 1.0 {
								newStrength = 1.0
							}
							if err := srv.store.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
								adjustments++
							}
							break
						}
					}
				}
				// Boost salience of returned memories
				for _, memID := range fb.RetrievedIDs {
					mem, err := srv.store.GetMemory(ctx, memID)
					if err != nil {
						continue
					}
					newSalience := mem.Salience + feedbackSalienceBoost
					if newSalience > 1.0 {
						newSalience = 1.0
					}
					if err := srv.store.UpdateSalience(ctx, memID, newSalience); err != nil {
						srv.log.Warn("failed to update salience", "memory_id", memID, "error", err)
					}
				}

			case "irrelevant":
				// Weaken traversed associations
				for _, ta := range fb.TraversedAssocs {
					assocs, err := srv.store.GetAssociations(ctx, ta.SourceID)
					if err != nil {
						continue
					}
					for _, a := range assocs {
						if a.TargetID == ta.TargetID {
							newStrength := a.Strength - feedbackStrengthDelta
							if newStrength < 0.05 {
								newStrength = 0.05
							}
							if err := srv.store.UpdateAssociationStrength(ctx, ta.SourceID, ta.TargetID, newStrength); err == nil {
								adjustments++
							}
							break
						}
					}
				}
			}
		}
	}

	srv.log.Info("feedback recorded", "query", query, "quality", quality, "query_id", queryID, "adjustments", adjustments)

	responseText := fmt.Sprintf("Feedback recorded: %s (query: %q)", quality, query)
	if adjustments > 0 {
		responseText += fmt.Sprintf(" — adjusted %d association strengths", adjustments)
	}

	return toolResult(responseText), nil
}

// handleAuditEncodings returns recent raw→encoded memory pairs for quality review.
func (srv *MCPServer) handleAuditEncodings(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	limit := 5
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
		if limit > 20 {
			limit = 20
		}
	}

	hoursBack := 24
	if h, ok := args["hours_back"].(float64); ok && int(h) > 0 {
		hoursBack = int(h)
	}

	sourceFilter := ""
	if s, ok := args["source"].(string); ok {
		sourceFilter = s
	}

	after := time.Now().Add(-time.Duration(hoursBack) * time.Hour)

	// Fetch recent raw memories — get extra to account for source filtering
	raws, err := srv.store.ListRawMemoriesAfter(ctx, after, limit*3)
	if err != nil {
		return nil, fmt.Errorf("failed to list raw memories: %w", err)
	}

	type auditPair struct {
		raw store.RawMemory
		mem *store.Memory
	}

	var pairs []auditPair
	for _, raw := range raws {
		if sourceFilter != "" && raw.Source != sourceFilter {
			continue
		}
		if len(pairs) >= limit {
			break
		}

		p := auditPair{raw: raw}

		// Look up the encoded memory by raw ID
		if mem, err := srv.store.GetMemoryByRawID(ctx, raw.ID); err == nil {
			p.mem = &mem
		}

		pairs = append(pairs, p)
	}

	if len(pairs) == 0 {
		return toolResult(fmt.Sprintf("No raw memories found in the last %d hours (source filter: %q).", hoursBack, sourceFilter)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Encoding Audit — last %dh, %d pair(s):\n\n", hoursBack, len(pairs)))

	for i, p := range pairs {
		sb.WriteString(fmt.Sprintf("--- Pair %d ---\n", i+1))
		sb.WriteString(fmt.Sprintf("RAW ID:      %s\n", p.raw.ID))
		sb.WriteString(fmt.Sprintf("Source:      %s\n", p.raw.Source))
		sb.WriteString(fmt.Sprintf("Type:        %s\n", p.raw.Type))
		sb.WriteString(fmt.Sprintf("Timestamp:   %s\n", p.raw.Timestamp.Format("2006-01-02 15:04:05")))

		rawContent := p.raw.Content
		if len(rawContent) > 300 {
			rawContent = rawContent[:300] + "..."
		}
		sb.WriteString(fmt.Sprintf("Raw Content: %s\n", rawContent))
		sb.WriteString("\n")

		if p.mem != nil {
			sb.WriteString(fmt.Sprintf("ENCODED ID:  %s\n", p.mem.ID))
			sb.WriteString(fmt.Sprintf("Summary:     %s\n", p.mem.Summary))
			sb.WriteString(fmt.Sprintf("Concepts:    %v\n", p.mem.Concepts))
			sb.WriteString(fmt.Sprintf("Salience:    %.2f\n", p.mem.Salience))
			sb.WriteString(fmt.Sprintf("Content:     %s\n", p.mem.Content))
			sb.WriteString(fmt.Sprintf("State:       %s\n", p.mem.State))
			sb.WriteString(fmt.Sprintf("AccessCount: %d\n", p.mem.AccessCount))
		} else {
			sb.WriteString("ENCODED:     (not yet encoded or encoding failed)\n")
		}
		sb.WriteString("\n")
	}

	srv.log.Info("audit_encodings completed", "pairs", len(pairs), "hours_back", hoursBack)
	return toolResult(sb.String()), nil
}

// handleCoachLocalLLM writes coaching instructions for the local LLM.
func (srv *MCPServer) handleCoachLocalLLM(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	coachingYAML, ok := args["coaching_yaml"].(string)
	if !ok || strings.TrimSpace(coachingYAML) == "" {
		return nil, fmt.Errorf("coaching_yaml parameter is required and must be a non-empty string")
	}

	// Validate: must be parseable YAML with a 'coaching' key
	var check map[string]interface{}
	if err := json.Unmarshal([]byte(coachingYAML), &check); err != nil {
		// Not JSON — try YAML parsing via a simpler check
		// We can't import yaml in mcp package easily, so validate structure via json roundtrip
		// Actually, just check that it contains "coaching:" as a basic validation
		if !strings.Contains(coachingYAML, "coaching:") {
			return nil, fmt.Errorf("coaching_yaml must contain a top-level 'coaching:' key")
		}
	} else {
		if _, ok := check["coaching"]; !ok {
			return nil, fmt.Errorf("coaching_yaml must have a top-level 'coaching' key")
		}
	}

	// Determine write path
	path := srv.coachingFile
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine coaching file path: %w", err)
		}
		path = home + "/.mnemonic/coaching.yaml"
	}

	// Ensure parent directory exists
	dir := path[:strings.LastIndex(path, "/")]
	if dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create coaching file directory: %w", err)
		}
	}

	// Write atomically: write to temp file, rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(coachingYAML), 0644); err != nil {
		return nil, fmt.Errorf("failed to write coaching file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		// Cleanup temp file on rename failure
		os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to finalize coaching file: %w", err)
	}

	srv.log.Info("coaching file written", "path", path)
	return toolResult(fmt.Sprintf(
		"Coaching file written to %s.\n\nNote: Restart the mnemonic daemon (`mnemonic restart`) for encoding agents to pick up the new coaching instructions.",
		path,
	)), nil
}

// handleIngestProject ingests a local directory into the memory system.
func (srv *MCPServer) handleIngestProject(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	directory, ok := args["directory"].(string)
	if !ok || directory == "" {
		return nil, fmt.Errorf("directory parameter is required and must be a string")
	}

	project := ""
	if p, ok := args["project"].(string); ok {
		project = p
	}

	dryRun := false
	if d, ok := args["dry_run"].(bool); ok {
		dryRun = d
	}

	cfg := ingest.Config{
		Dir:             directory,
		Project:         project,
		DryRun:          dryRun,
		ExcludePatterns: srv.excludePatterns,
		MaxContentBytes: srv.maxContentBytes,
	}

	result, err := ingest.Run(ctx, cfg, srv.store, srv.bus, srv.log)
	if err != nil {
		return nil, fmt.Errorf("ingest failed: %w", err)
	}

	if dryRun {
		return toolResult(fmt.Sprintf(
			"Dry run: found %d files in %s (project: %s). Nothing written.",
			result.FilesFound, directory, result.Project,
		)), nil
	}

	text := fmt.Sprintf(
		"Ingested %s (project: %s)\n\n"+
			"  Files found: %d\n"+
			"  Files written: %d\n"+
			"  Duplicates skipped: %d\n"+
			"  Files skipped (binary/empty): %d\n"+
			"  Files failed: %d\n"+
			"  Elapsed: %s",
		directory, result.Project,
		result.FilesFound, result.FilesWritten,
		result.DuplicatesSkipped, result.FilesSkipped,
		result.FilesFailed, result.Elapsed.Round(time.Millisecond),
	)

	if result.FilesWritten > 0 {
		encodeEstimate := result.FilesWritten * 8
		text += fmt.Sprintf("\n\n  The daemon will encode these over the next ~%d minutes.", encodeEstimate/60)
	}

	srv.log.Info("ingest completed via MCP",
		"directory", directory,
		"project", result.Project,
		"files_written", result.FilesWritten)

	return toolResult(text), nil
}

// Helper functions

// errorResponse creates a JSON-RPC error response.
func errorResponse(id interface{}, code int, message string) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: message,
		},
	}
}

// successResponse creates a JSON-RPC success response.
func successResponse(id interface{}, result interface{}) *jsonRPCResponse {
	return &jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
}

// toolResult creates an MCP tool result with text content.
func toolResult(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": text,
			},
		},
	}
}

// toolError creates an MCP tool error result.
func toolError(text string) map[string]interface{} {
	return map[string]interface{}{
		"content": []map[string]interface{}{
			{
				"type": "text",
				"text": fmt.Sprintf("Error: %s", text),
			},
		},
		"isError": true,
	}
}
