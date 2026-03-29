package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"io"
	"net/http"

	"github.com/appsprout-dev/mnemonic/internal/agent/retrieval"
	"github.com/appsprout-dev/mnemonic/internal/concepts"
	"github.com/appsprout-dev/mnemonic/internal/events"
	"github.com/appsprout-dev/mnemonic/internal/ingest"
	"github.com/appsprout-dev/mnemonic/internal/store"
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

// ProjectResolver resolves paths and names to canonical project names.
type ProjectResolver interface {
	Resolve(input string) string
}

// MemoryDefaults holds shared salience and feedback tuning values.
type MemoryDefaults struct {
	SalienceGeneral       float32
	SalienceDecision      float32
	SalienceError         float32
	SalienceInsight       float32
	SalienceLearning      float32
	SalienceHandoff       float32
	FeedbackStrengthDelta float32
	FeedbackSalienceBoost float32
}

// SalienceForType returns the initial salience for a given memory type.
func (d MemoryDefaults) SalienceForType(memType string) float32 {
	switch memType {
	case "decision":
		return d.SalienceDecision
	case "error":
		return d.SalienceError
	case "insight":
		return d.SalienceInsight
	case "learning":
		return d.SalienceLearning
	case "handoff":
		return d.SalienceHandoff
	default:
		return d.SalienceGeneral
	}
}

// DefaultMemoryDefaults returns the built-in defaults (used when no config override).
func DefaultMemoryDefaults() MemoryDefaults {
	return MemoryDefaults{
		SalienceGeneral:       0.7,
		SalienceDecision:      0.85,
		SalienceError:         0.8,
		SalienceInsight:       0.9,
		SalienceLearning:      0.8,
		SalienceHandoff:       0.95,
		FeedbackStrengthDelta: 0.05,
		FeedbackSalienceBoost: 0.02,
	}
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
	resolver        ProjectResolver
	coachingFile    string // path for coach_local_llm writes
	excludePatterns []string
	maxContentBytes int
	memDefaults     MemoryDefaults // shared salience and feedback tuning

	// Proactive context state (session-scoped)
	lastContextTime    time.Time       // watermark for get_context polling
	sessionRecalledIDs map[string]bool // memory IDs already surfaced via recall this session

	// Suggestion acceptance tracking (session-scoped)
	contextSuggestedIDs map[string]time.Time // memory IDs suggested by get_context → when
	contextAccepted     int                  // count of suggested IDs later recalled/rated
	contextTotalOffered int                  // total IDs offered across all get_context calls
	lastSuggestedIDsCSV string               // comma-separated IDs from last get_context (for tool_usage recording)

	// Daemon activity sync (for context_boost in MCP processes)
	daemonURL string // base URL of daemon API (e.g. "http://127.0.0.1:9999")
}

// NewMCPServer creates a new MCP server with the given dependencies.
func NewMCPServer(s store.Store, r *retrieval.RetrievalAgent, bus events.Bus, log *slog.Logger, version string, coachingFile string, excludePatterns []string, maxContentBytes int, resolver ProjectResolver, daemonURL string, memDefaults MemoryDefaults) *MCPServer {
	// Auto-detect project from working directory
	wd, _ := os.Getwd()
	var project string
	if resolver != nil {
		project = resolver.Resolve(wd)
	}
	if project == "" {
		project = detectProject()
	}

	// Generate session ID for this MCP server lifetime
	sessionID := fmt.Sprintf("mcp-%s", uuid.New().String()[:8])

	log.Info("MCP server initialized", "session_id", sessionID, "project", project)

	return &MCPServer{
		store:               s,
		retriever:           r,
		bus:                 bus,
		log:                 log,
		version:             version,
		sessionID:           sessionID,
		project:             project,
		resolver:            resolver,
		coachingFile:        coachingFile,
		excludePatterns:     excludePatterns,
		maxContentBytes:     maxContentBytes,
		memDefaults:         memDefaults,
		daemonURL:           daemonURL,
		lastContextTime:     time.Now(),
		sessionRecalledIDs:  make(map[string]bool),
		contextSuggestedIDs: make(map[string]time.Time),
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

// resolveProjectName resolves a user-supplied project name through the resolver.
func (srv *MCPServer) resolveProjectName(name string) string {
	if srv.resolver != nil && name != "" {
		if resolved := srv.resolver.Resolve(name); resolved != "" {
			return resolved
		}
	}
	return name
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

	// Session ended (stdin closed or context cancelled)
	srv.onSessionEnd(ctx)

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
	result := map[string]interface{}{
		"tools": allToolDefs(),
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

	start := time.Now()
	var result interface{}
	var toolErr error

	switch params.Name {
	case "remember":
		result, toolErr = srv.handleRemember(ctx, params.Arguments)
	case "recall":
		result, toolErr = srv.handleRecall(ctx, params.Arguments)
	case "batch_recall":
		result, toolErr = srv.handleBatchRecall(ctx, params.Arguments)
	case "get_context":
		result, toolErr = srv.handleGetContext(ctx, params.Arguments)
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
	case "list_sessions":
		result, toolErr = srv.handleListSessions(ctx, params.Arguments)
	case "recall_session":
		result, toolErr = srv.handleRecallSession(ctx, params.Arguments)
	case "exclude_path":
		result, toolErr = srv.handleExcludePath(ctx, params.Arguments)
	case "list_exclusions":
		result, toolErr = srv.handleListExclusions(ctx, params.Arguments)
	case "amend":
		result, toolErr = srv.handleAmend(ctx, params.Arguments)
	case "check_memory":
		result, toolErr = srv.handleCheckMemory(ctx, params.Arguments)
	case "dismiss_pattern":
		result, toolErr = srv.handleDismissPattern(ctx, params.Arguments)
	case "create_handoff":
		result, toolErr = srv.handleCreateHandoff(ctx, params.Arguments)
	default:
		return errorResponse(req.ID, -32602, fmt.Sprintf("Unknown tool: %s", params.Name))
	}

	// Record tool usage metrics
	srv.recordToolUsage(ctx, params, start, result, toolErr)

	if toolErr != nil {
		return successResponse(req.ID, toolError(toolErr.Error()))
	}

	return successResponse(req.ID, result)
}

// recordToolUsage logs metrics for an MCP tool invocation.
func (srv *MCPServer) recordToolUsage(ctx context.Context, params toolCallParams, start time.Time, result interface{}, toolErr error) {
	rec := store.ToolUsageRecord{
		Timestamp: start,
		ToolName:  params.Name,
		SessionID: srv.sessionID,
		Project:   srv.project,
		LatencyMs: time.Since(start).Milliseconds(),
		Success:   toolErr == nil,
	}
	if toolErr != nil {
		rec.ErrorMessage = toolErr.Error()
	}

	// Extract tool-specific context from arguments
	switch params.Name {
	case "recall", "recall_project", "recall_timeline":
		if q, ok := params.Arguments["query"].(string); ok {
			rec.QueryText = q
		}
	case "remember":
		if t, ok := params.Arguments["type"].(string); ok {
			rec.MemoryType = t
		}
	case "feedback":
		if r, ok := params.Arguments["quality"].(string); ok {
			rec.Rating = r
		}
	case "get_context":
		rec.SuggestedIDs = srv.lastSuggestedIDsCSV
	}

	// Measure response size and track get_context acceptance.
	if result != nil {
		if respBytes, err := json.Marshal(result); err == nil {
			rec.ResponseSize = len(respBytes)
		}
	}

	// Track suggestion acceptance: if a recall or feedback call references
	// memory IDs that get_context previously suggested, count as accepted.
	if len(srv.contextSuggestedIDs) > 0 {
		switch params.Name {
		case "recall", "recall_project", "recall_timeline", "recall_session", "batch_recall":
			srv.checkAcceptance(result)
		case "feedback":
			if ids, ok := params.Arguments["memory_ids"]; ok {
				if idList, ok := ids.([]interface{}); ok {
					for _, id := range idList {
						if idStr, ok := id.(string); ok {
							if _, suggested := srv.contextSuggestedIDs[idStr]; suggested {
								srv.contextAccepted++
								delete(srv.contextSuggestedIDs, idStr)
							}
						}
					}
				}
			}
		}
	}

	if err := srv.store.RecordToolUsage(ctx, rec); err != nil {
		srv.log.Warn("failed to record tool usage", "tool", params.Name, "error", err)
	}
}

// checkAcceptance scans a recall result for memory IDs that were previously
// suggested by get_context and marks them as accepted.
func (srv *MCPServer) checkAcceptance(result interface{}) {
	// The result is a toolResult map with "content" containing text.
	// Memory IDs appear as UUIDs in the text output — scan for matches.
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return
	}
	resultStr := string(resultBytes)
	for id := range srv.contextSuggestedIDs {
		if strings.Contains(resultStr, id) {
			srv.contextAccepted++
			delete(srv.contextSuggestedIDs, id)
		}
	}
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
		project = srv.resolveProjectName(p)
	}

	// Parse optional explicit associations.
	var explicitAssoc []map[string]string
	var invalidAssocIDs []string
	if rawAssoc, ok := args["associate_with"].([]interface{}); ok {
		for _, entry := range rawAssoc {
			if m, ok := entry.(map[string]interface{}); ok {
				memID, _ := m["memory_id"].(string)
				relation, _ := m["relation"].(string)
				if memID != "" && relation != "" {
					// Validate that the target memory exists.
					if _, err := srv.store.GetMemory(ctx, memID); err != nil {
						invalidAssocIDs = append(invalidAssocIDs, memID)
						continue
					}
					explicitAssoc = append(explicitAssoc, map[string]string{
						"memory_id": memID,
						"relation":  relation,
					})
				}
			}
		}
	}

	metadata := map[string]interface{}{
		"mcp_session_id": srv.sessionID,
		"memory_type":    memType,
		"project":        project,
	}
	if len(explicitAssoc) > 0 {
		metadata["explicit_associations"] = explicitAssoc
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
		Metadata:        metadata,
	}

	// Boost salience for specific types
	raw.InitialSalience = srv.memDefaults.SalienceForType(memType)

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

	msg := fmt.Sprintf("Stored memory %s (type: %s, project: %s)\n  Raw ID: %s\n  Initial salience: %.2f\n  Encoding: queued (async)\n\nTip: Use check_memory with raw_id %q to verify encoding status. Dedup protections: same-type, same-project, source-aware thresholds.",
		raw.ID, memType, project, raw.ID, raw.InitialSalience, raw.ID)
	if len(invalidAssocIDs) > 0 {
		msg += fmt.Sprintf("\n\nWarning: %d association target(s) not found and skipped: %s",
			len(invalidAssocIDs), strings.Join(invalidAssocIDs, ", "))
	}
	return toolResult(msg), nil
}

// syncActivityFromDaemon fetches the daemon's watcher activity tracker state
// and loads it into the local retrieval agent. This bridges the gap between
// the daemon process (which runs watchers) and the MCP process (which doesn't).
// Errors are logged but never block recall.
func (srv *MCPServer) syncActivityFromDaemon() {
	if srv.daemonURL == "" {
		return
	}
	resp, err := http.Get(srv.daemonURL + "/api/v1/activity")
	if err != nil {
		srv.log.Debug("activity sync failed", "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return
	}
	var body struct {
		Concepts map[string]time.Time `json:"concepts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		srv.log.Debug("activity sync decode failed", "error", err)
		return
	}
	if len(body.Concepts) > 0 {
		srv.retriever.SyncActivity(body.Concepts)
	}
}

// handleRecall retrieves memories using semantic search and spread activation.
// All recall paths (project-scoped, concept-filtered, default) go through the
// retrieval agent for spread activation and synthesis.
func (srv *MCPServer) handleRecall(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	query, ok := args["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("query parameter is required and must be a string")
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok {
		limit = int(l)
	}

	// Parse optional filters
	project := ""
	if p, ok := args["project"].(string); ok {
		project = p
	}

	source := ""
	if s, ok := args["source"].(string); ok {
		source = s
	}

	state := ""
	if s, ok := args["state"].(string); ok {
		state = s
	}

	memType := ""
	if t, ok := args["type"].(string); ok {
		memType = t
	}

	var minSalience float32
	if ms, ok := args["min_salience"].(float64); ok {
		minSalience = float32(ms)
	}

	var concepts []string
	if c, ok := args["concepts"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				concepts = append(concepts, s)
			}
		}
	}

	var excludeConcepts []string
	if c, ok := args["exclude_concepts"].([]interface{}); ok {
		for _, v := range c {
			if s, ok := v.(string); ok {
				excludeConcepts = append(excludeConcepts, s)
			}
		}
	}

	explain := false
	if e, ok := args["explain"].(bool); ok {
		explain = e
	}

	includeAssociations := false
	if ia, ok := args["include_associations"].(bool); ok {
		includeAssociations = ia
	}

	synthesize := false
	if s, ok := args["synthesize"].(bool); ok {
		synthesize = s
	}

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	// If concepts are specified, use concept-based search (no spread activation available)
	if len(concepts) > 0 {
		memories, err := srv.store.SearchByConcepts(ctx, concepts, limit)
		if err != nil {
			srv.log.Error("concept recall failed", "concepts", concepts, "error", err)
			return nil, fmt.Errorf("concept recall failed: %w", err)
		}
		filtered := filterMemories(memories, source, state, memType, minSalience)
		if len(excludeConcepts) > 0 {
			var kept []store.Memory
			for _, m := range filtered {
				if !conceptOverlap(m.Concepts, excludeConcepts) {
					kept = append(kept, m)
				}
			}
			filtered = kept
		}
		text := fmt.Sprintf("Found %d memories matching concepts %v:\n\n", len(filtered), concepts)
		for i, mem := range filtered {
			text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n\n",
				i+1, mem.ID, mem.Summary, mem.Concepts)
		}
		return toolResult(text), nil
	}

	// All other queries go through the retrieval agent (including project-scoped)
	queryReq := retrieval.QueryRequest{
		Query:               query,
		MaxResults:          limit,
		IncludeReasoning:    true,
		Synthesize:          synthesize,
		IncludePatterns:     true,
		IncludeAbstractions: true,
		Project:             project,
		Source:              source,
		State:               state,
		Type:                memType,
		MinSalience:         minSalience,
		ExcludeConcepts:     excludeConcepts,
	}

	result, err := srv.retriever.Query(ctx, queryReq)
	if err != nil {
		srv.log.Error("retrieval failed", "query", query, "error", err)
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// Filter patterns and abstractions by exclude_concepts
	if len(excludeConcepts) > 0 {
		var filteredPatterns []store.Pattern
		for _, p := range result.Patterns {
			if !conceptOverlap(p.Concepts, excludeConcepts) {
				filteredPatterns = append(filteredPatterns, p)
			}
		}
		result.Patterns = filteredPatterns

		var filteredAbstractions []store.Abstraction
		for _, a := range result.Abstractions {
			if !conceptOverlap(a.Concepts, excludeConcepts) {
				filteredAbstractions = append(filteredAbstractions, a)
			}
		}
		result.Abstractions = filteredAbstractions
	}

	// Save traversal data and access snapshot for feedback loop
	var retrievedIDs []string
	var snapshot []store.AccessSnapshotEntry
	for i, mem := range result.Memories {
		retrievedIDs = append(retrievedIDs, mem.Memory.ID)
		snapshot = append(snapshot, store.AccessSnapshotEntry{
			MemoryID: mem.Memory.ID,
			Rank:     i + 1,
			Score:    mem.Score,
		})
	}
	fb := store.RetrievalFeedback{
		QueryID:         result.QueryID,
		QueryText:       query,
		RetrievedIDs:    retrievedIDs,
		TraversedAssocs: result.TraversedAssocs,
		AccessSnapshot:  snapshot,
		CreatedAt:       time.Now(),
	}
	// Track recalled IDs for proactive context dedup.
	for _, id := range retrievedIDs {
		srv.sessionRecalledIDs[id] = true
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
		explanationInfo := ""
		if explain && mem.Explanation != "" {
			explanationInfo = fmt.Sprintf("\n   Explanation: %s", mem.Explanation)
		}
		associationInfo := ""
		if includeAssociations {
			assocs, aErr := srv.store.GetAssociations(ctx, mem.Memory.ID)
			if aErr == nil && len(assocs) > 0 {
				limit := 3
				if len(assocs) < limit {
					limit = len(assocs)
				}
				associationInfo = "\n   Related:"
				for j := 0; j < limit; j++ {
					a := assocs[j]
					targetSummary := a.TargetID[:8]
					if tm, tErr := srv.store.GetMemory(ctx, a.TargetID); tErr == nil {
						targetSummary = tm.Summary
						if len(targetSummary) > 80 {
							targetSummary = targetSummary[:80] + "..."
						}
					}
					associationInfo += fmt.Sprintf("\n     - [%.2f, %s] %s", a.Strength, a.RelationType, targetSummary)
				}
			}
		}
		text += fmt.Sprintf("%d. [%.3f] %s\n   Summary: %s%s\n   Concepts: %v\n   Created: %s%s%s%s\n\n",
			i+1, mem.Score, mem.Memory.ID, mem.Memory.Summary, contentSnippet,
			mem.Memory.Concepts, mem.Memory.CreatedAt.Format("2006-01-02 15:04"), projectInfo, explanationInfo, associationInfo)
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

	if outputFormat == "json" {
		var assocMap map[string][]store.Association
		if includeAssociations {
			assocMap = make(map[string][]store.Association, len(result.Memories))
			for _, m := range result.Memories {
				assocs, aErr := srv.store.GetAssociations(ctx, m.Memory.ID)
				if aErr == nil {
					assocMap[m.Memory.ID] = assocs
				}
			}
		}
		jsonResp := formatRecallJSON(result, assocMap)
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult(text), nil // fallback to text
		}
		return toolResult(string(jsonBytes)), nil
	}

	return toolResult(text), nil
}

// formatRecallJSON builds a structured map from retrieval results.
// assocMap is optional — when non-nil, associations are included per memory.
func formatRecallJSON(result retrieval.QueryResponse, assocMap map[string][]store.Association) map[string]interface{} {
	memories := make([]map[string]interface{}, len(result.Memories))
	for i, m := range result.Memories {
		memories[i] = map[string]interface{}{
			"id":           m.Memory.ID,
			"raw_id":       m.Memory.RawID,
			"score":        m.Score,
			"summary":      m.Memory.Summary,
			"content":      m.Memory.Content,
			"concepts":     m.Memory.Concepts,
			"source":       m.Memory.Source,
			"type":         m.Memory.Type,
			"project":      m.Memory.Project,
			"salience":     m.Memory.Salience,
			"state":        m.Memory.State,
			"access_count": m.Memory.AccessCount,
			"session_id":   m.Memory.SessionID,
			"created_at":   m.Memory.CreatedAt,
			"explanation":  m.Explanation,
		}
		if assocMap != nil {
			if assocs, ok := assocMap[m.Memory.ID]; ok && len(assocs) > 0 {
				limit := 3
				if len(assocs) < limit {
					limit = len(assocs)
				}
				jsonAssocs := make([]map[string]interface{}, limit)
				for j := 0; j < limit; j++ {
					a := assocs[j]
					jsonAssocs[j] = map[string]interface{}{
						"target_id":     a.TargetID,
						"strength":      a.Strength,
						"relation_type": a.RelationType,
					}
				}
				memories[i]["associations"] = jsonAssocs
			}
		}
	}

	patterns := make([]map[string]interface{}, len(result.Patterns))
	for i, p := range result.Patterns {
		patterns[i] = map[string]interface{}{
			"title":       p.Title,
			"type":        p.PatternType,
			"strength":    p.Strength,
			"description": p.Description,
		}
	}

	abstractions := make([]map[string]interface{}, len(result.Abstractions))
	for i, a := range result.Abstractions {
		abstractions[i] = map[string]interface{}{
			"title":       a.Title,
			"level":       a.Level,
			"confidence":  a.Confidence,
			"description": a.Description,
		}
	}

	return map[string]interface{}{
		"query_id":     result.QueryID,
		"memories":     memories,
		"patterns":     patterns,
		"abstractions": abstractions,
		"synthesis":    result.Synthesis,
		"took_ms":      result.TookMs,
	}
}

// formatMemoriesJSON builds a JSON array from a slice of memories (for recall_project, recall_timeline, recall_session).
func formatMemoriesJSON(memories []store.Memory) []map[string]interface{} {
	result := make([]map[string]interface{}, len(memories))
	for i, m := range memories {
		result[i] = map[string]interface{}{
			"id":           m.ID,
			"raw_id":       m.RawID,
			"summary":      m.Summary,
			"content":      m.Content,
			"concepts":     m.Concepts,
			"source":       m.Source,
			"type":         m.Type,
			"project":      m.Project,
			"salience":     m.Salience,
			"state":        m.State,
			"access_count": m.AccessCount,
			"session_id":   m.SessionID,
			"created_at":   m.CreatedAt,
		}
	}
	return result
}

// formatPatternsJSON builds a JSON array from a slice of patterns.
func formatPatternsJSON(patterns []store.Pattern) []map[string]interface{} {
	result := make([]map[string]interface{}, len(patterns))
	for i, p := range patterns {
		result[i] = map[string]interface{}{
			"title":       p.Title,
			"type":        p.PatternType,
			"strength":    p.Strength,
			"description": p.Description,
			"concepts":    p.Concepts,
			"project":     p.Project,
		}
	}
	return result
}

// handleBatchRecall runs multiple recall queries in parallel and returns combined JSON results.
func (srv *MCPServer) handleBatchRecall(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	queriesRaw, ok := args["queries"].([]interface{})
	if !ok || len(queriesRaw) == 0 {
		return nil, fmt.Errorf("queries parameter is required and must be a non-empty array")
	}

	if len(queriesRaw) > 10 {
		return nil, fmt.Errorf("maximum 10 queries per batch (got %d)", len(queriesRaw))
	}

	type batchResult struct {
		Index int
		Query string
		Data  map[string]interface{}
		Err   error
	}

	results := make(chan batchResult, len(queriesRaw))

	for i, qRaw := range queriesRaw {
		qMap, ok := qRaw.(map[string]interface{})
		if !ok {
			results <- batchResult{Index: i, Err: fmt.Errorf("query %d: invalid format", i)}
			continue
		}

		query, _ := qMap["query"].(string)
		if query == "" {
			results <- batchResult{Index: i, Err: fmt.Errorf("query %d: query string is required", i)}
			continue
		}

		go func(idx int, q string, qArgs map[string]interface{}) {
			limit := 5
			if l, ok := qArgs["limit"].(float64); ok {
				limit = int(l)
			}
			project := ""
			if p, ok := qArgs["project"].(string); ok {
				project = p
			}
			source := ""
			if s, ok := qArgs["source"].(string); ok {
				source = s
			}
			memType := ""
			if t, ok := qArgs["type"].(string); ok {
				memType = t
			}
			var minSalience float32
			if ms, ok := qArgs["min_salience"].(float64); ok {
				minSalience = float32(ms)
			}

			qr, err := srv.retriever.Query(ctx, retrieval.QueryRequest{
				Query:               q,
				MaxResults:          limit,
				IncludeReasoning:    true,
				IncludePatterns:     true,
				IncludeAbstractions: true,
				Project:             project,
				Source:              source,
				Type:                memType,
				MinSalience:         minSalience,
			})
			if err != nil {
				results <- batchResult{Index: idx, Query: q, Err: err}
				return
			}

			results <- batchResult{
				Index: idx,
				Query: q,
				Data:  formatRecallJSON(qr, nil),
			}
		}(i, query, qMap)
	}

	// Collect results in order.
	collected := make([]map[string]interface{}, len(queriesRaw))
	for range queriesRaw {
		r := <-results
		if r.Err != nil {
			collected[r.Index] = map[string]interface{}{
				"query": r.Query,
				"error": r.Err.Error(),
			}
		} else {
			r.Data["query"] = r.Query
			collected[r.Index] = r.Data
		}
	}

	jsonBytes, err := json.Marshal(map[string]interface{}{
		"results": collected,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling batch results: %w", err)
	}

	srv.log.Info("batch recall completed", "queries", len(queriesRaw))
	return toolResult(string(jsonBytes)), nil
}

// handleGetContext returns proactive memory suggestions based on recent daemon activity.
// It reads recent watcher events from the DB, extracts concepts, and finds related
// encoded memories the agent hasn't already recalled this session.
func (srv *MCPServer) handleGetContext(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sinceMinutes := 10
	if m, ok := args["since_minutes"].(float64); ok && int(m) > 0 {
		sinceMinutes = int(m)
	}

	limit := 5
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	// Use watermark if available, otherwise use since_minutes.
	since := srv.lastContextTime
	sinceOverride := time.Now().Add(-time.Duration(sinceMinutes) * time.Minute)
	if sinceOverride.Before(since) {
		since = sinceOverride
	}

	// Step 1: Fetch recent raw memories (watcher activity).
	raws, err := srv.store.ListRawMemoriesAfter(ctx, since, 50)
	if err != nil {
		return nil, fmt.Errorf("listing recent activity: %w", err)
	}

	// Filter to current project if set, and exclude MCP source (agent's own memories).
	var relevant []store.RawMemory
	for _, raw := range raws {
		if raw.Source == "mcp" {
			continue // Skip agent's own memories — we want daemon observations.
		}
		if srv.project != "" && raw.Project != "" && raw.Project != srv.project {
			continue
		}
		relevant = append(relevant, raw)
	}

	if len(relevant) == 0 {
		srv.lastContextTime = time.Now()
		return toolResult("No recent activity detected. The daemon watcher hasn't observed new events since your last check."), nil
	}

	// Step 2: Extract concepts from recent activity, tracking encoding coverage.
	conceptCounts := make(map[string]int)
	var encodedCount, fallbackCount int
	var encodeLats []float64 // encode latencies in ms for encoded memories
	for _, raw := range relevant {
		// Prefer encoded memory concepts if available.
		mem, err := srv.store.GetMemoryByRawID(ctx, raw.ID)
		var extracted []string
		if err == nil && len(mem.Concepts) > 0 {
			extracted = mem.Concepts
			encodedCount++
			// Track encoding latency: time from raw creation to encoded memory creation.
			if !mem.CreatedAt.IsZero() && !raw.CreatedAt.IsZero() {
				latMs := float64(mem.CreatedAt.Sub(raw.CreatedAt).Milliseconds())
				if latMs >= 0 {
					encodeLats = append(encodeLats, latMs)
				}
			}
		} else if raw.Source == "filesystem" {
			// For filesystem events, extract concepts from the file path
			// instead of content — raw content is source code whose tokens
			// (Go keywords, type names, etc.) pollute theme extraction.
			if pathVal, ok := raw.Metadata["path"].(string); ok && pathVal != "" {
				extracted = concepts.FromPath(pathVal)
			}
			// Enrich with the event action (created, modified, deleted).
			if action := concepts.FromEventType(raw.Type); action != "" {
				extracted = append(extracted, action)
			}
			fallbackCount++
		} else if raw.Source == "terminal" {
			// For terminal events, extract command name and subcommand
			// rather than treating the full command as natural language.
			extracted = concepts.FromCommand(raw.Content)
			fallbackCount++
		} else {
			extracted = retrieval.ParseQueryConcepts(raw.Content)
			fallbackCount++
		}
		for _, c := range extracted {
			conceptCounts[c]++
		}
	}

	if len(conceptCounts) == 0 {
		srv.lastContextTime = time.Now()
		return toolResult("Recent activity detected but no meaningful concepts extracted."), nil
	}

	// Step 3: Rank concepts by frequency, take top 8.
	type conceptFreq struct {
		concept string
		count   int
	}
	var ranked []conceptFreq
	for c, n := range conceptCounts {
		ranked = append(ranked, conceptFreq{c, n})
	}
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].count > ranked[j].count
	})
	topN := 8
	if len(ranked) < topN {
		topN = len(ranked)
	}
	topConcepts := make([]string, topN)
	for i := 0; i < topN; i++ {
		topConcepts[i] = ranked[i].concept
	}

	// Step 4: Search for related encoded memories.
	candidates, err := srv.store.SearchByConceptsInProject(ctx, topConcepts, srv.project, limit*3)
	if err != nil {
		srv.log.Warn("proactive context search failed", "error", err)
		candidates = nil
	}

	// Step 5: Filter — exclude already-recalled, suppressed, archived, low-match.
	// Track all passing candidates (before limit) for novelty metrics.
	var suggestions []store.Memory
	var allPassing int
	for _, mem := range candidates {
		if srv.sessionRecalledIDs[mem.ID] {
			continue
		}
		if mem.RecallSuppressed {
			continue
		}
		if mem.State == "archived" {
			continue
		}
		// Require at least 2 concept matches.
		matches := 0
		for _, mc := range mem.Concepts {
			if conceptCounts[mc] > 0 {
				matches++
			}
		}
		if matches < 2 {
			continue
		}
		allPassing++
		if len(suggestions) < limit {
			suggestions = append(suggestions, mem)
		}
	}

	// Compute theme match counts: for each top concept, how many suggestions have it.
	themeHits := make(map[string]int, len(topConcepts))
	for _, tc := range topConcepts {
		for _, mem := range suggestions {
			for _, mc := range mem.Concepts {
				if mc == tc {
					themeHits[tc]++
					break
				}
			}
		}
	}

	// Compute encoding queue depth and oldest unencoded age.
	var queueDepth int
	var oldestUnencoded string
	unprocessed, listErr := srv.store.ListRawUnprocessed(ctx, 1000)
	if listErr == nil {
		queueDepth = len(unprocessed)
		if queueDepth > 0 {
			oldest := unprocessed[len(unprocessed)-1]
			age := time.Since(oldest.CreatedAt)
			oldestUnencoded = formatDuration(age)
		}
	}

	// Compute average encode latency.
	var avgEncodeLat float64
	if len(encodeLats) > 0 {
		var sum float64
		for _, l := range encodeLats {
			sum += l
		}
		avgEncodeLat = sum / float64(len(encodeLats))
	}

	// Compute novelty rate.
	var noveltyPct float64
	if len(candidates) > 0 {
		noveltyPct = float64(allPassing) / float64(len(candidates)) * 100
	}

	// Compute encoding coverage.
	var coveragePct float64
	if len(relevant) > 0 {
		coveragePct = float64(encodedCount) / float64(len(relevant)) * 100
	}

	// Compute acceptance rate from prior suggestions.
	var acceptancePct float64
	if srv.contextTotalOffered > 0 {
		acceptancePct = float64(srv.contextAccepted) / float64(srv.contextTotalOffered) * 100
	}

	// Build metrics.
	metrics := contextMetrics{
		EncodedCount:     encodedCount,
		FallbackCount:    fallbackCount,
		CoveragePct:      coveragePct,
		CandidatesBefore: len(candidates),
		CandidatesAfter:  allPassing,
		NoveltyPct:       noveltyPct,
		ThemeHits:        themeHits,
		AvgEncodeLatMs:   avgEncodeLat,
		OldestUnencoded:  oldestUnencoded,
		QueueDepth:       queueDepth,
		AcceptancePct:    acceptancePct,
	}

	// Track suggested IDs for acceptance measurement and tool_usage recording.
	var suggestedIDs []string
	for _, mem := range suggestions {
		srv.contextSuggestedIDs[mem.ID] = time.Now()
		suggestedIDs = append(suggestedIDs, mem.ID)
	}
	srv.contextTotalOffered += len(suggestions)
	srv.lastSuggestedIDsCSV = strings.Join(suggestedIDs, ",")

	// Step 6: Update watermark.
	srv.lastContextTime = time.Now()

	srv.log.Info("proactive context generated",
		"recent_events", len(relevant),
		"themes", topConcepts,
		"suggestions", len(suggestions),
		"encoding_coverage_pct", coveragePct,
		"novelty_pct", noveltyPct,
		"queue_depth", queueDepth)

	// Format output.
	if outputFormat == "json" {
		jsonResp := map[string]interface{}{
			"recent_events": len(relevant),
			"themes":        topConcepts,
			"suggestions":   formatMemoriesJSON(suggestions),
			"metrics":       metrics,
		}
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult("json marshal error"), nil
		}
		return toolResult(string(jsonBytes)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Recent activity: %d events since last check\n", len(relevant))
	fmt.Fprintf(&sb, "Activity themes: %v\n\n", topConcepts)

	if len(suggestions) == 0 {
		sb.WriteString("No new context suggestions — you've already recalled the relevant memories.\n")
	} else {
		fmt.Fprintf(&sb, "Suggested context (%d memories you haven't recalled):\n\n", len(suggestions))
		for i, mem := range suggestions {
			fmt.Fprintf(&sb, "%d. %s\n   Summary: %s\n   Concepts: %v\n   Created: %s\n\n",
				i+1, mem.ID, mem.Summary, mem.Concepts,
				mem.CreatedAt.Format("2006-01-02"))
		}
	}

	// Pipeline metrics footer.
	sb.WriteString("--- Pipeline ---\n")
	fmt.Fprintf(&sb, "Coverage: %.0f%% encoded (%d/%d)", coveragePct, encodedCount, len(relevant))
	if queueDepth > 0 {
		fmt.Fprintf(&sb, " | Queue: %d pending, oldest %s ago", queueDepth, oldestUnencoded)
	}
	sb.WriteString("\n")
	if len(candidates) > 0 {
		fmt.Fprintf(&sb, "Candidates: %d -> %d after dedup (%.0f%% novel)\n", len(candidates), allPassing, noveltyPct)
	}
	if len(themeHits) > 0 {
		sb.WriteString("Theme hits: [")
		first := true
		for _, tc := range topConcepts {
			if n, ok := themeHits[tc]; ok && n > 0 {
				if !first {
					sb.WriteString(", ")
				}
				fmt.Fprintf(&sb, "%s:%d", tc, n)
				first = false
			}
		}
		sb.WriteString("]\n")
	}
	if srv.contextTotalOffered > 0 {
		fmt.Fprintf(&sb, "Acceptance: %.0f%% (%d/%d suggestions led to recall)\n",
			acceptancePct, srv.contextAccepted, srv.contextTotalOffered)
	}

	return toolResult(sb.String()), nil
}

// contextMetrics holds pipeline observability data for the get_context tool.
type contextMetrics struct {
	EncodedCount     int            `json:"encoded_count"`
	FallbackCount    int            `json:"fallback_count"`
	CoveragePct      float64        `json:"encoding_coverage_pct"`
	CandidatesBefore int            `json:"candidates_before_dedup"`
	CandidatesAfter  int            `json:"candidates_after_dedup"`
	NoveltyPct       float64        `json:"novelty_pct"`
	ThemeHits        map[string]int `json:"theme_match_counts"`
	AvgEncodeLatMs   float64        `json:"avg_encode_latency_ms"`
	OldestUnencoded  string         `json:"oldest_unencoded_age"`
	QueueDepth       int            `json:"encoding_queue_depth"`
	AcceptancePct    float64        `json:"acceptance_rate_pct"`
}

// formatDuration returns a human-readable short duration string.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

// handleForget archives a memory by ID.
func (srv *MCPServer) handleForget(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Collect IDs from memory_id (string) and/or memory_ids (array).
	var ids []string
	if singleID, ok := args["memory_id"].(string); ok && singleID != "" {
		ids = append(ids, singleID)
	}
	if rawIDs, ok := args["memory_ids"].([]interface{}); ok {
		for _, raw := range rawIDs {
			if id, ok := raw.(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("either memory_id or memory_ids is required")
	}

	// Deduplicate.
	seen := make(map[string]bool, len(ids))
	var unique []string
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}

	var archived, failed int
	var failedIDs []string
	for _, id := range unique {
		if err := srv.store.UpdateState(ctx, id, "archived"); err != nil {
			srv.log.Warn("failed to archive memory", "id", id, "error", err)
			failed++
			failedIDs = append(failedIDs, id)
		} else {
			archived++
		}
	}

	srv.log.Info("memories archived", "archived", archived, "failed", failed)

	msg := fmt.Sprintf("Archived %d memories", archived)
	if failed > 0 {
		msg += fmt.Sprintf(", %d failed: %v", failed, failedIDs)
	}
	return toolResult(msg), nil
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

	// Retrieval quality summary from recent feedback
	feedbacks, err := srv.store.ListMetaObservations(ctx, "retrieval_feedback", 50)
	if err == nil && len(feedbacks) > 0 {
		helpful, partial, irrelevant := 0, 0, 0
		for _, fb := range feedbacks {
			if q, ok := fb.Details["quality"].(string); ok {
				switch q {
				case "helpful":
					helpful++
				case "partial":
					partial++
				case "irrelevant":
					irrelevant++
				}
			}
		}
		total := helpful + partial + irrelevant
		if total > 0 {
			text += fmt.Sprintf("\nRetrieval quality (last %d feedbacks):\n", total)
			text += fmt.Sprintf("  Helpful: %d (%.0f%%), Partial: %d (%.0f%%), Irrelevant: %d (%.0f%%)\n",
				helpful, float64(helpful)/float64(total)*100,
				partial, float64(partial)/float64(total)*100,
				irrelevant, float64(irrelevant)/float64(total)*100)
			if float64(irrelevant)/float64(total) > 0.3 {
				text += "  ⚠ High irrelevant rate — feedback is driving association adjustments\n"
			}
		}
	}

	// Pipeline health: pending encodings
	pendingRaws, pendingErr := srv.store.ListRawUnprocessed(ctx, 100)
	if pendingErr == nil {
		text += "\nEncoding pipeline:\n"
		text += fmt.Sprintf("  Pending: %d raw memories awaiting encoding\n", len(pendingRaws))
		if len(pendingRaws) > 0 {
			oldest := pendingRaws[len(pendingRaws)-1] // last in priority-sorted list = oldest/lowest priority
			text += fmt.Sprintf("  Oldest pending: %s (%s, source: %s)\n", oldest.ID[:8], oldest.CreatedAt.Format("2006-01-02 15:04"), oldest.Source)
			// Count by source
			srcCounts := make(map[string]int)
			for _, r := range pendingRaws {
				srcCounts[r.Source]++
			}
			for src, count := range srcCounts {
				text += fmt.Sprintf("    %s: %d\n", src, count)
			}
		}
	}

	// Source distribution
	srcDist, srcErr := srv.store.GetSourceDistribution(ctx)
	if srcErr == nil && len(srcDist) > 0 {
		text += "\nMemory sources:\n"
		for src, count := range srcDist {
			text += fmt.Sprintf("  %s: %d\n", src, count)
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
// Routes through the retrieval agent for spread activation and synthesis.
func (srv *MCPServer) handleRecallProject(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	srv.syncActivityFromDaemon()

	project := srv.project
	if p, ok := args["project"].(string); ok && p != "" {
		project = srv.resolveProjectName(p)
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

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	// Parse optional filters — default min_salience to 0.7 for project recall
	// to filter out watcher noise that agents don't need.
	source, state, memType, minSalience := parseRecallFilters(args)
	if _, explicit := args["min_salience"]; !explicit && minSalience == 0 {
		minSalience = 0.7
	}

	// Get project summary
	summary, err := srv.store.GetProjectSummary(ctx, project)
	if err != nil {
		srv.log.Warn("failed to get project summary", "project", project, "error", err)
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

	// Filter patterns to quality threshold
	if len(patterns) > 0 {
		filtered := patterns[:0]
		for _, p := range patterns {
			if p.Strength >= 0.3 {
				filtered = append(filtered, p)
			}
		}
		patterns = filtered
	}
	if len(patterns) > 0 {
		text += fmt.Sprintf("\nPatterns (%d):\n", len(patterns))
		for _, p := range patterns {
			text += fmt.Sprintf("  - [%.2f] %s: %s\n", p.Strength, p.Title, p.Description)
		}
	}

	// Collect memories from either the retrieval agent or recent project search.
	var resultMemories []store.Memory
	var synthesis string

	if query != "" {
		queryReq := retrieval.QueryRequest{
			Query:               query,
			MaxResults:          limit,
			IncludeReasoning:    true,
			Synthesize:          true,
			IncludePatterns:     false, // already fetched above
			IncludeAbstractions: true,
			Project:             project,
			Source:              source,
			State:               state,
			Type:                memType,
			MinSalience:         minSalience,
		}

		result, err := srv.retriever.Query(ctx, queryReq)
		if err != nil {
			srv.log.Error("project recall failed", "project", project, "error", err)
			return nil, fmt.Errorf("project recall failed: %w", err)
		}

		for _, mem := range result.Memories {
			resultMemories = append(resultMemories, mem.Memory)
		}
		synthesis = result.Synthesis
	} else {
		memories, err := srv.store.SearchByProject(ctx, project, "", limit)
		if err != nil {
			srv.log.Error("project recall failed", "project", project, "error", err)
			return nil, fmt.Errorf("project recall failed: %w", err)
		}
		resultMemories = filterMemories(memories, source, state, memType, minSalience)
	}

	srv.log.Info("project recall completed", "project", project)

	if outputFormat == "json" {
		jsonResp := map[string]interface{}{
			"project":  project,
			"summary":  summary,
			"patterns": formatPatternsJSON(patterns),
			"memories": formatMemoriesJSON(resultMemories),
		}
		if synthesis != "" {
			jsonResp["synthesis"] = synthesis
		}
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult(text), nil
		}
		return toolResult(string(jsonBytes)), nil
	}

	// Text output.
	text += fmt.Sprintf("\nMemories (%d):\n\n", len(resultMemories))
	for i, mem := range resultMemories {
		text += fmt.Sprintf("%d. %s\n   Summary: %s\n   Concepts: %v\n   State: %s\n\n",
			i+1, mem.ID, mem.Summary, mem.Concepts, mem.State)
	}
	if synthesis != "" {
		text += fmt.Sprintf("\nSynthesis:\n%s\n", synthesis)
	}

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

	source, state, memType, minSalience := parseRecallFilters(args)

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	from := time.Now().Add(-time.Duration(hoursBack) * time.Hour)
	to := time.Now()

	memories, err := srv.store.ListMemoriesByTimeRange(ctx, from, to, limit)
	if err != nil {
		srv.log.Error("timeline recall failed", "error", err)
		return nil, fmt.Errorf("timeline recall failed: %w", err)
	}

	filtered := filterMemories(memories, source, state, memType, minSalience)

	srv.log.Info("timeline recall completed", "hours_back", hoursBack, "memories", len(filtered))

	if outputFormat == "json" {
		jsonResp := map[string]interface{}{
			"hours_back": hoursBack,
			"memories":   formatMemoriesJSON(filtered),
		}
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult("json marshal error"), nil
		}
		return toolResult(string(jsonBytes)), nil
	}

	text := fmt.Sprintf("Timeline (last %dh, %d memories):\n\n", hoursBack, len(filtered))
	for i, mem := range filtered {
		projectInfo := ""
		if mem.Project != "" {
			projectInfo = fmt.Sprintf(" [%s]", mem.Project)
		}
		text += fmt.Sprintf("%d. %s%s\n   %s\n   Concepts: %v\n\n",
			i+1, mem.Timestamp.Format("2006-01-02 15:04:05"), projectInfo,
			mem.Summary, mem.Concepts)
	}

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

	minStrength := float32(0.3)
	if ms, ok := args["min_strength"].(float64); ok {
		minStrength = float32(ms)
	}

	patterns, err := srv.store.ListPatterns(ctx, project, limit)
	if err != nil {
		srv.log.Error("failed to list patterns", "error", err)
		return nil, fmt.Errorf("failed to list patterns: %w", err)
	}

	// Filter by minimum strength
	if minStrength > 0 {
		filtered := patterns[:0]
		for _, p := range patterns {
			if p.Strength >= minStrength {
				filtered = append(filtered, p)
			}
		}
		patterns = filtered
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

// srv.memDefaults.FeedbackStrengthDelta and srv.memDefaults.FeedbackSalienceBoost are now on srv.memDefaults.

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
							newStrength := a.Strength + srv.memDefaults.FeedbackStrengthDelta
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
					newSalience := mem.Salience + srv.memDefaults.FeedbackSalienceBoost
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
							newStrength := a.Strength - srv.memDefaults.FeedbackStrengthDelta
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

	// Update per-memory feedback scores for auto-suppression.
	// Uses memoryIDs from the feedback call (the memories that were returned).
	suppressionThreshold := -3
	suppressed := 0
	unsuppressed := 0
	for _, memID := range memoryIDs {
		mem, err := srv.store.GetMemory(ctx, memID)
		if err != nil {
			continue
		}
		switch quality {
		case "helpful":
			mem.FeedbackScore++
			// Lift suppression if a helpful rating comes in
			if mem.RecallSuppressed {
				mem.RecallSuppressed = false
				unsuppressed++
			}
		case "irrelevant":
			mem.FeedbackScore--
		}
		// Check suppression threshold
		if mem.FeedbackScore <= suppressionThreshold && !mem.RecallSuppressed {
			mem.RecallSuppressed = true
			suppressed++
		}
		mem.UpdatedAt = time.Now()
		if err := srv.store.UpdateMemory(ctx, mem); err != nil {
			srv.log.Warn("failed to update memory feedback score", "memory_id", memID, "error", err)
		}
	}

	srv.log.Info("feedback recorded", "query", query, "quality", quality, "query_id", queryID, "adjustments", adjustments)

	responseText := fmt.Sprintf("Feedback recorded: %s (query: %q)", quality, query)
	if adjustments > 0 {
		responseText += fmt.Sprintf(" — adjusted %d association strengths", adjustments)
	}
	if suppressed > 0 {
		responseText += fmt.Sprintf(", suppressed %d memories from future recall", suppressed)
	}
	if unsuppressed > 0 {
		responseText += fmt.Sprintf(", un-suppressed %d memories", unsuppressed)
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
	fmt.Fprintf(&sb, "Encoding Audit — last %dh, %d pair(s):\n\n", hoursBack, len(pairs))

	for i, p := range pairs {
		fmt.Fprintf(&sb, "--- Pair %d ---\n", i+1)
		fmt.Fprintf(&sb, "RAW ID:      %s\n", p.raw.ID)
		fmt.Fprintf(&sb, "Source:      %s\n", p.raw.Source)
		fmt.Fprintf(&sb, "Type:        %s\n", p.raw.Type)
		fmt.Fprintf(&sb, "Timestamp:   %s\n", p.raw.Timestamp.Format("2006-01-02 15:04:05"))

		rawContent := p.raw.Content
		if len(rawContent) > 300 {
			rawContent = rawContent[:300] + "..."
		}
		fmt.Fprintf(&sb, "Raw Content: %s\n", rawContent)
		sb.WriteString("\n")

		if p.mem != nil {
			fmt.Fprintf(&sb, "ENCODED ID:  %s\n", p.mem.ID)
			fmt.Fprintf(&sb, "Summary:     %s\n", p.mem.Summary)
			fmt.Fprintf(&sb, "Concepts:    %v\n", p.mem.Concepts)
			fmt.Fprintf(&sb, "Salience:    %.2f\n", p.mem.Salience)
			fmt.Fprintf(&sb, "Content:     %s\n", p.mem.Content)
			fmt.Fprintf(&sb, "State:       %s\n", p.mem.State)
			fmt.Fprintf(&sb, "AccessCount: %d\n", p.mem.AccessCount)
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
		_ = os.Remove(tmpPath)
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
			"  Documents extracted: %d\n"+
			"  Document chunks created: %d\n"+
			"  Duplicates skipped: %d\n"+
			"  Files skipped (binary/empty): %d\n"+
			"  Files failed: %d\n"+
			"  Elapsed: %s",
		directory, result.Project,
		result.FilesFound, result.FilesWritten,
		result.FilesExtracted, result.ChunksCreated,
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
		"files_written", result.FilesWritten,
		"files_extracted", result.FilesExtracted,
		"chunks_created", result.ChunksCreated)

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

// parseRecallFilters extracts optional source/state/min_salience from MCP args.
func parseRecallFilters(args map[string]interface{}) (source, state, memType string, minSalience float32) {
	if s, ok := args["source"].(string); ok {
		source = s
	}
	if s, ok := args["state"].(string); ok {
		state = s
	}
	if t, ok := args["type"].(string); ok {
		memType = t
	}
	if ms, ok := args["min_salience"].(float64); ok {
		minSalience = float32(ms)
	}
	return
}

// filterMemories filters a slice of memories by source, state, and minimum salience.
func filterMemories(memories []store.Memory, source, state, memType string, minSalience float32) []store.Memory {
	var filtered []store.Memory
	for _, m := range memories {
		if source != "" && m.Source != source {
			continue
		}
		if state != "" && m.State != state {
			continue
		}
		if memType != "" && m.Type != memType {
			continue
		}
		if minSalience > 0 && m.Salience < minSalience {
			continue
		}
		if m.RecallSuppressed {
			continue
		}
		filtered = append(filtered, m)
	}
	return filtered
}

// conceptOverlap returns true if any memory concept matches any excluded concept (case-insensitive).
func conceptOverlap(memoryConcepts, excluded []string) bool {
	for _, mc := range memoryConcepts {
		for _, ec := range excluded {
			if strings.EqualFold(mc, ec) {
				return true
			}
		}
	}
	return false
}

// onSessionEnd is called when stdin closes (Claude Code disconnected) or context is cancelled.
// It records session metadata so future sessions can see what happened.
func (srv *MCPServer) onSessionEnd(ctx context.Context) {
	srv.log.Info("MCP session ending", "session_id", srv.sessionID, "project", srv.project)

	// Count memories created during this session
	memories, err := srv.store.ListMemoriesBySession(ctx, srv.sessionID)
	memCount := 0
	if err == nil {
		memCount = len(memories)
	}

	// Record session end as a meta observation
	obs := store.MetaObservation{
		ID:              fmt.Sprintf("session-end-%s", srv.sessionID),
		ObservationType: "session_end",
		Severity:        "info",
		Details: map[string]interface{}{
			"session_id":       srv.sessionID,
			"project":          srv.project,
			"memories_created": memCount,
			"ended_at":         time.Now().Format(time.RFC3339),
		},
		CreatedAt: time.Now(),
	}
	if err := srv.store.WriteMetaObservation(ctx, obs); err != nil {
		srv.log.Warn("failed to write session end observation", "error", err)
	}

	// Publish session end event for other agents to react to
	if srv.bus != nil {
		_ = srv.bus.Publish(ctx, events.SessionEnded{
			SessionID: srv.sessionID,
			Project:   srv.project,
			Ts:        time.Now(),
		})
	}

	srv.log.Info("MCP session ended", "session_id", srv.sessionID, "memories_created", memCount)
}

// handleListSessions returns recent MCP sessions with metadata.
func (srv *MCPServer) handleListSessions(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	limit := 10
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	daysBack := 30
	if d, ok := args["days_back"].(float64); ok && int(d) > 0 {
		daysBack = int(d)
	}

	since := time.Now().AddDate(0, 0, -daysBack)
	sessions, err := srv.store.ListSessions(ctx, since, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sessions: %w", err)
	}

	if len(sessions) == 0 {
		return toolResult("No sessions found."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Found %d sessions:\n\n", len(sessions))
	for i, s := range sessions {
		fmt.Fprintf(&sb, "%d. %s\n   Time: %s to %s\n   Memories: %d\n\n",
			i+1, s.SessionID,
			s.StartTime.Format("2006-01-02 15:04"),
			s.EndTime.Format("2006-01-02 15:04"),
			s.MemoryCount)
	}
	return toolResult(sb.String()), nil
}

// handleRecallSession retrieves all memories from a specific session.
func (srv *MCPServer) handleRecallSession(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	sessionID, ok := args["session_id"].(string)
	if !ok || sessionID == "" {
		return nil, fmt.Errorf("session_id parameter is required")
	}

	// Allow "current" to resolve to the active MCP session.
	if sessionID == "current" {
		sessionID = srv.sessionID
	}

	limit := 20
	if l, ok := args["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}

	outputFormat := "text"
	if f, ok := args["format"].(string); ok && f == "json" {
		outputFormat = f
	}

	memories, err := srv.store.GetSessionMemories(ctx, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("getting session memories: %w", err)
	}

	if len(memories) == 0 {
		if outputFormat == "json" {
			jsonBytes, _ := json.Marshal(map[string]interface{}{
				"session_id": sessionID,
				"memories":   []interface{}{},
			})
			return toolResult(string(jsonBytes)), nil
		}
		return toolResult(fmt.Sprintf("No memories found for session %s.", sessionID)), nil
	}

	if outputFormat == "json" {
		jsonResp := map[string]interface{}{
			"session_id": sessionID,
			"memories":   formatMemoriesJSON(memories),
		}
		jsonBytes, err := json.Marshal(jsonResp)
		if err != nil {
			return toolResult("json marshal error"), nil
		}
		return toolResult(string(jsonBytes)), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Session %s (%d memories):\n\n", sessionID, len(memories))
	for i, mem := range memories {
		fmt.Fprintf(&sb, "%d. [%s] %s\n   Summary: %s\n   Concepts: %v\n   Type: %s\n\n",
			i+1, mem.CreatedAt.Format("15:04:05"), mem.ID, mem.Summary, mem.Concepts, mem.Type)
	}
	return toolResult(sb.String()), nil
}

// handleExcludePath adds a watcher exclusion pattern to the DB.
func (srv *MCPServer) handleExcludePath(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	pattern, ok := args["pattern"].(string)
	if !ok || pattern == "" {
		return nil, fmt.Errorf("pattern parameter is required")
	}

	if err := srv.store.AddRuntimeExclusion(ctx, pattern); err != nil {
		return nil, fmt.Errorf("adding exclusion: %w", err)
	}

	srv.log.Info("runtime exclusion added", "pattern", pattern)
	return toolResult(fmt.Sprintf("Added exclusion pattern %q. Takes effect on daemon restart.", pattern)), nil
}

// handleListExclusions returns all runtime watcher exclusion patterns.
func (srv *MCPServer) handleListExclusions(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	patterns, err := srv.store.ListRuntimeExclusions(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing exclusions: %w", err)
	}

	if len(patterns) == 0 {
		return toolResult("No runtime exclusions configured. Use exclude_path to add patterns."), nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Runtime exclusions (%d):\n", len(patterns))
	for _, p := range patterns {
		fmt.Fprintf(&sb, "  - %s\n", p)
	}
	return toolResult(sb.String()), nil
}

// handleAmend updates a memory's content in place, preserving associations and history.
func (srv *MCPServer) handleAmend(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	memoryID, ok := args["memory_id"].(string)
	if !ok || memoryID == "" {
		return nil, fmt.Errorf("memory_id parameter is required")
	}

	correctedContent, ok := args["corrected_content"].(string)
	if !ok || correctedContent == "" {
		return nil, fmt.Errorf("corrected_content parameter is required")
	}

	// Generate a simple summary (first 120 chars of content)
	summary := correctedContent
	if len(summary) > 120 {
		summary = summary[:120] + "..."
	}

	// Use empty concepts and embedding — encoding agent can re-process if needed
	if err := srv.store.AmendMemory(ctx, memoryID, correctedContent, summary, nil, nil); err != nil {
		srv.log.Error("failed to amend memory", "memory_id", memoryID, "error", err)
		return nil, fmt.Errorf("failed to amend memory: %w", err)
	}

	// Publish event
	if srv.bus != nil {
		_ = srv.bus.Publish(ctx, events.MemoryAmended{
			MemoryID:   memoryID,
			NewSummary: summary,
			Ts:         time.Now(),
		})
	}

	srv.log.Info("memory amended", "memory_id", memoryID)
	return toolResult(fmt.Sprintf("Amended memory %s. Content updated, associations and history preserved. Salience bumped +0.05.", memoryID)), nil
}

// handleCheckMemory inspects a memory's encoding status, concepts, and associations.
func (srv *MCPServer) handleCheckMemory(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	rawID, _ := args["raw_id"].(string)
	memoryID, _ := args["memory_id"].(string)

	if rawID == "" && memoryID == "" {
		return nil, fmt.Errorf("at least one of raw_id or memory_id is required")
	}

	// Try to find the encoded memory
	var mem store.Memory
	var found bool

	if memoryID != "" {
		m, err := srv.store.GetMemory(ctx, memoryID)
		if err == nil {
			mem = m
			found = true
		}
	}

	if !found && rawID != "" {
		m, err := srv.store.GetMemoryByRawID(ctx, rawID)
		if err == nil {
			mem = m
			found = true
		}
	}

	if !found {
		// Check if the raw memory exists but hasn't been encoded yet
		if rawID != "" {
			raw, err := srv.store.GetRaw(ctx, rawID)
			if err != nil {
				return toolResult(fmt.Sprintf("No memory found for raw_id %q or memory_id %q.", rawID, memoryID)), nil
			}
			status := "pending encoding"
			if raw.Processed {
				status = "deduplicated — a similar memory already existed, so this one boosted its salience instead of creating a duplicate"
			}
			return toolResult(fmt.Sprintf("Raw memory %s found but not yet encoded.\n  Status: %s\n  Source: %s\n  Type: %s\n  Salience: %.2f\n  Created: %s",
				raw.ID, status, raw.Source, raw.Type, raw.InitialSalience, raw.CreatedAt.Format(time.RFC3339))), nil
		}
		return toolResult(fmt.Sprintf("No memory found for memory_id %q.", memoryID)), nil
	}

	// Get associations
	assocs, err := srv.store.GetAssociations(ctx, mem.ID)
	if err != nil {
		assocs = nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Memory %s (encoded)\n", mem.ID)
	fmt.Fprintf(&sb, "  Raw ID: %s\n", mem.RawID)
	fmt.Fprintf(&sb, "  Summary: %s\n", mem.Summary)
	fmt.Fprintf(&sb, "  Concepts: %v\n", mem.Concepts)
	fmt.Fprintf(&sb, "  Salience: %.2f\n", mem.Salience)
	fmt.Fprintf(&sb, "  State: %s\n", mem.State)
	fmt.Fprintf(&sb, "  Access count: %d\n", mem.AccessCount)
	fmt.Fprintf(&sb, "  Source: %s\n", mem.Source)
	fmt.Fprintf(&sb, "  Type: %s\n", mem.Type)
	fmt.Fprintf(&sb, "  Created: %s\n", mem.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "  Associations: %d\n", len(assocs))

	for i, a := range assocs {
		if i >= 5 {
			fmt.Fprintf(&sb, "  ... and %d more\n", len(assocs)-5)
			break
		}
		targetMem, err := srv.store.GetMemory(ctx, a.TargetID)
		summary := a.TargetID
		if err == nil {
			summary = targetMem.Summary
			if len(summary) > 80 {
				summary = summary[:80] + "..."
			}
		}
		fmt.Fprintf(&sb, "    [%.2f, %s] %s\n", a.Strength, a.RelationType, summary)
	}

	return toolResult(sb.String()), nil
}

// handleDismissPattern archives a pattern by ID.
func (srv *MCPServer) handleDismissPattern(_ context.Context, args map[string]interface{}) (interface{}, error) {
	patternID, _ := args["pattern_id"].(string)
	if patternID == "" {
		return nil, fmt.Errorf("pattern_id is required")
	}

	if err := srv.store.ArchivePattern(context.Background(), patternID); err != nil {
		return nil, fmt.Errorf("archiving pattern %s: %w", patternID, err)
	}

	srv.log.Info("pattern dismissed", "pattern_id", patternID, "session_id", srv.sessionID)
	return toolResult(fmt.Sprintf("Pattern %s archived", patternID)), nil
}

// handleCreateHandoff stores a structured session handoff note as a high-salience memory.
func (srv *MCPServer) handleCreateHandoff(ctx context.Context, args map[string]interface{}) (interface{}, error) {
	// Parse all fields.
	var completed, pending, toTest, knownIssues []string
	for _, pair := range []struct {
		key  string
		dest *[]string
	}{
		{"completed", &completed},
		{"pending", &pending},
		{"to_test", &toTest},
		{"known_issues", &knownIssues},
	} {
		if raw, ok := args[pair.key].([]interface{}); ok {
			for _, v := range raw {
				if s, ok := v.(string); ok && s != "" {
					*pair.dest = append(*pair.dest, s)
				}
			}
		}
	}
	nextHint, _ := args["next_session_hint"].(string)

	if len(completed) == 0 && len(pending) == 0 && len(toTest) == 0 && len(knownIssues) == 0 && nextHint == "" {
		return nil, fmt.Errorf("at least one field must be provided")
	}

	// Format as readable text.
	var sb strings.Builder
	fmt.Fprintf(&sb, "SESSION HANDOFF — %s — %s\n\n", srv.project, time.Now().Format("2006-01-02 15:04"))
	writeSection := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&sb, "%s:\n", title)
		for _, item := range items {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
		sb.WriteString("\n")
	}
	writeSection("Completed", completed)
	writeSection("Pending", pending)
	writeSection("To Test", toTest)
	writeSection("Known Issues", knownIssues)
	if nextHint != "" {
		fmt.Fprintf(&sb, "Next session: %s\n", nextHint)
	}

	raw := store.RawMemory{
		ID:              uuid.New().String(),
		Source:          "mcp",
		Type:            "handoff",
		Content:         sb.String(),
		Timestamp:       time.Now(),
		CreatedAt:       time.Now(),
		HeuristicScore:  0.9,
		InitialSalience: srv.memDefaults.SalienceForType("handoff"),
		Processed:       false,
		Project:         srv.project,
		SessionID:       srv.sessionID,
		Metadata: map[string]interface{}{
			"mcp_session_id":    srv.sessionID,
			"memory_type":       "handoff",
			"project":           srv.project,
			"completed":         completed,
			"pending":           pending,
			"to_test":           toTest,
			"known_issues":      knownIssues,
			"next_session_hint": nextHint,
		},
	}

	if err := srv.store.WriteRaw(ctx, raw); err != nil {
		return nil, fmt.Errorf("failed to store handoff: %w", err)
	}
	if srv.bus != nil {
		_ = srv.bus.Publish(ctx, events.RawMemoryCreated{
			ID:     raw.ID,
			Source: raw.Source,
			Ts:     time.Now(),
		})
	}

	srv.log.Info("session handoff created", "id", raw.ID, "project", srv.project)
	return toolResult(fmt.Sprintf("Handoff stored (id: %s, salience: 0.95)\nWill be surfaced by recall_project in the next session.", raw.ID)), nil
}
