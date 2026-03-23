"""Canonical tool list definitions for the mnemonic-agent.

Single source of truth — imported by options.py and subagents.py.
No internal SDK imports to avoid circular dependencies.
"""
from __future__ import annotations

BUILTIN_TOOLS = [
    "Read",
    "Edit",
    "Write",
    "Bash",
    "Glob",
    "Grep",
    "Task",
]

ALL_MNEMONIC_TOOLS = [
    "mcp__mnemonic__remember",
    "mcp__mnemonic__recall",
    "mcp__mnemonic__batch_recall",
    "mcp__mnemonic__forget",
    "mcp__mnemonic__amend",
    "mcp__mnemonic__check_memory",
    "mcp__mnemonic__status",
    "mcp__mnemonic__recall_project",
    "mcp__mnemonic__recall_timeline",
    "mcp__mnemonic__recall_session",
    "mcp__mnemonic__list_sessions",
    "mcp__mnemonic__session_summary",
    "mcp__mnemonic__get_context",
    "mcp__mnemonic__get_patterns",
    "mcp__mnemonic__get_insights",
    "mcp__mnemonic__feedback",
    "mcp__mnemonic__audit_encodings",
    "mcp__mnemonic__coach_local_llm",
    "mcp__mnemonic__ingest_project",
    "mcp__mnemonic__exclude_path",
    "mcp__mnemonic__list_exclusions",
    "mcp__mnemonic__dismiss_pattern",
    "mcp__mnemonic__create_handoff",
]

MNEMONIC_RECALL_TOOLS = [
    "mcp__mnemonic__recall",
    "mcp__mnemonic__recall_project",
    "mcp__mnemonic__remember",
    "mcp__mnemonic__get_patterns",
    "mcp__mnemonic__get_insights",
    "mcp__mnemonic__feedback",
]
