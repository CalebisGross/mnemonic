from __future__ import annotations

from claude_agent_sdk import ClaudeAgentOptions, HookMatcher

from agent.config import Config
from agent.hooks import post_tool_use_hook
from agent.prompts import assemble_system_prompt
from agent.subagents import make_subagents
from agent.tools import ALL_MNEMONIC_TOOLS, BUILTIN_TOOLS


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

    # Disable extended thinking — the web UI doesn't display it and it
    # adds latency / cost for content that is silently discarded.
    options_kwargs["thinking"] = {"type": "disabled"}

    if cfg.max_turns is not None:
        options_kwargs["max_turns"] = cfg.max_turns

    if cfg.resume:
        options_kwargs["resume"] = cfg.resume

    return ClaudeAgentOptions(**options_kwargs)
