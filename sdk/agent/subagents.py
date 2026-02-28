from __future__ import annotations

from claude_agent_sdk import AgentDefinition

MNEMONIC_RECALL_TOOLS = [
    "mcp__mnemonic__recall",
    "mcp__mnemonic__recall_project",
    "mcp__mnemonic__remember",
    "mcp__mnemonic__get_patterns",
    "mcp__mnemonic__get_insights",
    "mcp__mnemonic__feedback",
]

ALL_MNEMONIC_TOOLS = MNEMONIC_RECALL_TOOLS + [
    "mcp__mnemonic__forget",
    "mcp__mnemonic__status",
    "mcp__mnemonic__recall_timeline",
    "mcp__mnemonic__session_summary",
]


def make_subagents(subagent_model: str = "sonnet") -> dict[str, AgentDefinition]:
    """Build subagent definitions with the specified model."""
    return {
        "code-reviewer": AgentDefinition(
            description=(
                "Reviews code for correctness, style, security, and patterns. "
                "Use when asked to review a file, PR diff, or specific function. "
                "Has access to the memory system for recalling past review patterns."
            ),
            prompt=(
                "You are a meticulous code reviewer with persistent memory.\n\n"
                "Before reviewing, call mcp__mnemonic__recall_project to load project context "
                "and mcp__mnemonic__recall with a query about past review issues.\n\n"
                "During review, note:\n"
                "- Security issues → mcp__mnemonic__remember type='error'\n"
                "- Style decisions → mcp__mnemonic__remember type='decision'\n"
                "- Recurring patterns → mcp__mnemonic__remember type='insight'\n\n"
                "Provide structured feedback: Summary, Issues (critical/warning/suggestion), Verdict."
            ),
            tools=["Read", "Grep", "Glob"] + MNEMONIC_RECALL_TOOLS,
            model=subagent_model,
        ),
        "test-runner": AgentDefinition(
            description=(
                "Runs the test suite and interprets results. "
                "Use when asked to run tests, check test coverage, or debug test failures."
            ),
            prompt=(
                "You are a test execution specialist with persistent memory.\n\n"
                "Before running tests, call mcp__mnemonic__recall with query='test failures flaky' "
                "to check for known issues.\n\n"
                "Run tests, capture output. If tests fail, diagnose the root cause.\n"
                "Store persistent failures as type='error' memories.\n"
                "Store test infrastructure discoveries as type='insight'."
            ),
            tools=["Bash", "Read", "Glob"] + MNEMONIC_RECALL_TOOLS,
            model=subagent_model,
        ),
        "memory-archivist": AgentDefinition(
            description=(
                "Queries and summarizes the Mnemonic memory system. "
                "Use when asked about past decisions, project history, patterns, "
                "or 'what did we work on last week'."
            ),
            prompt=(
                "You are a memory curator. Retrieve and synthesize memories from Mnemonic.\n\n"
                "Use recall_project for project overviews, recall for specific queries, "
                "recall_timeline for chronological questions, get_patterns for recurring themes, "
                "and get_insights for high-level abstractions.\n\n"
                "Present findings in clear, structured prose."
            ),
            tools=ALL_MNEMONIC_TOOLS,
            model=subagent_model,
        ),
    }
