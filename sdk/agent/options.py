from __future__ import annotations

from claude_agent_sdk import ClaudeAgentOptions, HookMatcher

from agent.config import Config
from agent.hooks import post_tool_use_hook
from agent.prompts import assemble_system_prompt
from agent.subagents import make_subagents

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
    "mcp__mnemonic__forget",
    "mcp__mnemonic__status",
    "mcp__mnemonic__recall_project",
    "mcp__mnemonic__recall_timeline",
    "mcp__mnemonic__session_summary",
    "mcp__mnemonic__get_patterns",
    "mcp__mnemonic__get_insights",
    "mcp__mnemonic__feedback",
    "mcp__mnemonic__audit_encodings",
    "mcp__mnemonic__coach_local_llm",
]


def build_options(cfg: Config) -> ClaudeAgentOptions:
    """Build ClaudeAgentOptions wiring Mnemonic MCP, tools, hooks, and subagents."""
    system_prompt_append = assemble_system_prompt(cfg.evolution_dir)

    mnemonic_server = {
        "type": "stdio",
        "command": cfg.mnemonic_binary,
        "args": ["--config", cfg.mnemonic_config, "mcp"],
        "cwd": cfg.project_root,
    }

    options_kwargs: dict = {
        "system_prompt": {
            "type": "preset",
            "preset": "claude_code",
            "append": system_prompt_append,
        },
        "mcp_servers": {"mnemonic": mnemonic_server},
        "allowed_tools": BUILTIN_TOOLS + ALL_MNEMONIC_TOOLS,
        "permission_mode": cfg.permission_mode,
        "cwd": cfg.project_cwd,
        "model": cfg.model,
        "hooks": {
            "PostToolUse": [
                HookMatcher(matcher=None, hooks=[post_tool_use_hook]),
            ],
        },
        "agents": make_subagents(cfg.subagent_model),
    }

    if cfg.max_turns is not None:
        options_kwargs["max_turns"] = cfg.max_turns

    return ClaudeAgentOptions(**options_kwargs)
