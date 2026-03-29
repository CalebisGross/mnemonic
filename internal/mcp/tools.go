package mcp

// Tool schema definitions for the MCP server.
// Each function returns a ToolDefinition describing one MCP tool.

func rememberToolDef() ToolDefinition {
	return ToolDefinition{
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
				"associate_with": map[string]interface{}{
					"type":        "array",
					"description": "Create explicit associations with existing memories at write time",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"memory_id": map[string]interface{}{
								"type":        "string",
								"description": "ID of the memory to associate with",
							},
							"relation": map[string]interface{}{
								"type":        "string",
								"description": "Relation type",
								"enum":        []string{"similar", "caused_by", "part_of", "contradicts", "temporal", "reinforces"},
							},
						},
						"required": []string{"memory_id", "relation"},
					},
				},
			},
			"required": []string{"text"},
		},
	}
}

func recallToolDef() ToolDefinition {
	return ToolDefinition{
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
				"exclude_concepts": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Exclude memories containing any of these concepts",
				},
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory source: mcp, filesystem, terminal, clipboard",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory type: decision, error, insight, learning, general",
					"enum":        []string{"decision", "error", "insight", "learning", "general"},
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Minimum salience threshold (0.0-1.0). Filters out low-quality memories.",
				},
				"state": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory state: active, fading, archived",
					"enum":        []string{"active", "fading", "archived"},
				},
				"explain": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, include score breakdown for each result (activation, recency, source weight, feedback adjustment)",
				},
				"include_associations": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, include top associated memories for each result (default: false)",
				},
				"synthesize": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, include LLM-generated synthesis narrative (default: false). Adds 3-8s latency.",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{"query"},
		},
	}
}

func batchRecallToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "batch_recall",
		Description: "Run multiple recall queries in a single request. Returns structured JSON results for each query. Ideal for session start when you need project context, prior decisions, and task-specific memories in one round-trip.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"queries": map[string]interface{}{
					"type":        "array",
					"description": "Array of recall queries to execute",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"query": map[string]interface{}{
								"type":        "string",
								"description": "The search query",
							},
							"limit": map[string]interface{}{
								"type":        "integer",
								"description": "Maximum results for this query (default: 5)",
							},
							"project": map[string]interface{}{
								"type":        "string",
								"description": "Filter by project name",
							},
							"source": map[string]interface{}{
								"type":        "string",
								"description": "Filter by memory source: mcp, filesystem, terminal, clipboard",
							},
							"type": map[string]interface{}{
								"type":        "string",
								"description": "Filter by memory type: decision, error, insight, learning, general",
							},
							"min_salience": map[string]interface{}{
								"type":        "number",
								"description": "Minimum salience threshold (0.0-1.0)",
							},
						},
						"required": []string{"query"},
					},
				},
			},
			"required": []string{"queries"},
		},
	}
}

func getContextToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "get_context",
		Description: "Get proactive memory suggestions based on recent daemon activity (file edits, terminal commands, clipboard). Call at natural breakpoints to discover relevant context you haven't recalled yet. Returns memories related to what you're currently working on without needing to formulate a query.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"since_minutes": map[string]interface{}{
					"type":        "integer",
					"description": "Look-back window in minutes (default: 10)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum suggestions to return (default: 5)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
	}
}

func forgetToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "forget",
		Description: "Archive one or more memories by ID. Supports single memory_id or bulk memory_ids array.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "Single memory ID to archive",
				},
				"memory_ids": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Array of memory IDs to archive in bulk",
				},
			},
		},
	}
}

func dismissPatternToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "dismiss_pattern",
		Description: "Archive a pattern by ID. Use this to dismiss stale or irrelevant patterns that keep surfacing in recall results.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the pattern to archive",
				},
			},
			"required": []string{"pattern_id"},
		},
	}
}

func statusToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "status",
		Description: "Get memory system statistics, health insights, and project breakdown",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
			"required":   []string{},
		},
	}
}

func recallProjectToolDef() ToolDefinition {
	return ToolDefinition{
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
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory source: mcp, filesystem, terminal, clipboard",
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Minimum salience threshold (0.0-1.0). Filters out low-quality memories.",
				},
				"state": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory state: active, fading, archived",
					"enum":        []string{"active", "fading", "archived"},
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
	}
}

func recallTimelineToolDef() ToolDefinition {
	return ToolDefinition{
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
				"source": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory source: mcp, filesystem, terminal, clipboard",
				},
				"min_salience": map[string]interface{}{
					"type":        "number",
					"description": "Minimum salience threshold (0.0-1.0). Filters out low-quality memories.",
				},
				"state": map[string]interface{}{
					"type":        "string",
					"description": "Filter by memory state: active, fading, archived",
					"enum":        []string{"active", "fading", "archived"},
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{},
		},
	}
}

func sessionSummaryToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func getPatternsToolDef() ToolDefinition {
	return ToolDefinition{
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
				"min_strength": map[string]interface{}{
					"type":        "number",
					"description": "Minimum pattern strength to return (default: 0.3). Set to 0 for all patterns.",
				},
			},
			"required": []string{},
		},
	}
}

func getInsightsToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func feedbackToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func auditEncodingsToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func coachLocalLLMToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func ingestProjectToolDef() ToolDefinition {
	return ToolDefinition{
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
	}
}

func listSessionsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "list_sessions",
		Description: "List recent MCP sessions with metadata (time range, memory count). Useful for finding a specific past session to recall.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum sessions to return (default: 10)",
				},
				"days_back": map[string]interface{}{
					"type":        "integer",
					"description": "How many days back to search (default: 30)",
				},
			},
		},
	}
}

func recallSessionToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "recall_session",
		Description: "Retrieve all memories from a specific MCP session, ordered by creation time. Use \"current\" for the active session, or list_sessions to find past session IDs.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"session_id": map[string]interface{}{
					"type":        "string",
					"description": "The session ID to recall memories from",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum memories to return (default: 20)",
				},
				"format": map[string]interface{}{
					"type":        "string",
					"description": "Output format: text (default) or json (structured data)",
					"enum":        []string{"text", "json"},
				},
			},
			"required": []string{"session_id"},
		},
	}
}

func excludePathToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "exclude_path",
		Description: "Add a path pattern to the watcher exclusion list. Prevents future watcher events from matching paths. Takes effect on daemon restart. Use list_exclusions to see current patterns.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Path substring to exclude (e.g., '.cache/', 'node_modules/', '/tmp/')",
				},
			},
			"required": []string{"pattern"},
		},
	}
}

func listExclusionsToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "list_exclusions",
		Description: "List all runtime watcher exclusion patterns (added via exclude_path).",
		InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func amendToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "amend",
		Description: "Update a memory's content while preserving its ID, associations, activation history, and salience. Use when a recalled memory is stale or incorrect. Records an audit trail of the change.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "The memory ID to amend",
				},
				"corrected_content": map[string]interface{}{
					"type":        "string",
					"description": "The updated memory content",
				},
			},
			"required": []string{"memory_id", "corrected_content"},
		},
	}
}

func checkMemoryToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "check_memory",
		Description: "Inspect a memory's encoding status, extracted concepts, associations, and current salience. Use raw_id (from remember) or memory_id to look up a specific memory.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"raw_id": map[string]interface{}{
					"type":        "string",
					"description": "The raw memory ID returned by remember",
				},
				"memory_id": map[string]interface{}{
					"type":        "string",
					"description": "The encoded memory ID",
				},
			},
		},
	}
}

func createHandoffToolDef() ToolDefinition {
	return ToolDefinition{
		Name:        "create_handoff",
		Description: "Create a structured session handoff note for the next session. Stored with high salience and automatically surfaced by recall_project. Use at session end to preserve continuity.",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"completed": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Tasks completed this session",
				},
				"pending": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Tasks started but not finished",
				},
				"to_test": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Items that need testing",
				},
				"known_issues": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Known bugs or issues discovered",
				},
				"next_session_hint": map[string]interface{}{
					"type":        "string",
					"description": "Suggested starting point for the next session",
				},
			},
		},
	}
}

// ToolCount returns the number of registered MCP tools.
func ToolCount() int {
	return len(allToolDefs())
}

// allToolDefs returns the complete list of MCP tool definitions.
func allToolDefs() []ToolDefinition {
	return []ToolDefinition{
		rememberToolDef(),
		recallToolDef(),
		batchRecallToolDef(),
		getContextToolDef(),
		forgetToolDef(),
		statusToolDef(),
		recallProjectToolDef(),
		recallTimelineToolDef(),
		sessionSummaryToolDef(),
		getPatternsToolDef(),
		getInsightsToolDef(),
		feedbackToolDef(),
		auditEncodingsToolDef(),
		coachLocalLLMToolDef(),
		ingestProjectToolDef(),
		listSessionsToolDef(),
		recallSessionToolDef(),
		excludePathToolDef(),
		listExclusionsToolDef(),
		amendToolDef(),
		checkMemoryToolDef(),
		dismissPatternToolDef(),
		createHandoffToolDef(),
	}
}
