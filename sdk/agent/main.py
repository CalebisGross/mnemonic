from __future__ import annotations

import argparse
import asyncio
import logging
import os
import sys

from agent.config import Config
from agent.session import run_session


def cli() -> None:
    parser = argparse.ArgumentParser(
        prog="mnemonic-agent",
        description="Self-evolving coding assistant powered by Claude + Mnemonic",
    )
    parser.add_argument(
        "task",
        nargs="?",
        help="Initial task prompt (if omitted, starts interactive REPL)",
    )
    parser.add_argument(
        "--cwd",
        default=os.getcwd(),
        help="Working directory for the agent (default: current directory)",
    )
    parser.add_argument(
        "--model",
        default=os.environ.get("MNEMONIC_AGENT_MODEL", "claude-sonnet-4-6"),
        help="Claude model to use (default: claude-sonnet-4-6)",
    )
    parser.add_argument(
        "--mnemonic-binary",
        default=None,
        help="Path to the mnemonic binary (default: auto-detect from sdk/../mnemonic)",
    )
    parser.add_argument(
        "--mnemonic-config",
        default=None,
        help="Path to mnemonic config.yaml (default: auto-detect from sdk/../config.yaml)",
    )
    parser.add_argument(
        "--permission-mode",
        choices=["default", "acceptEdits", "bypassPermissions"],
        default="acceptEdits",
        help="Tool permission mode (default: acceptEdits)",
    )
    parser.add_argument(
        "--evolve-interval",
        type=int,
        default=5,
        help="Tasks between evolution cycles (default: 5)",
    )
    parser.add_argument(
        "--max-turns",
        type=int,
        default=None,
        help="Maximum agentic turns per query",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Show tool calls, thinking, and system messages",
    )
    parser.add_argument(
        "--no-reflect",
        action="store_true",
        help="Skip pre/post task reflection and evolution (faster, no memory overhead)",
    )
    parser.add_argument(
        "--subagent-model",
        default="sonnet",
        help="Model for subagents: code-reviewer, test-runner, memory-archivist (default: sonnet)",
    )

    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(message)s",
    )

    cfg = Config(
        project_cwd=args.cwd,
        model=args.model,
        permission_mode=args.permission_mode,
        evolve_interval=args.evolve_interval,
        max_turns=args.max_turns,
        verbose=args.verbose,
        no_reflect=args.no_reflect,
        subagent_model=args.subagent_model,
    )

    # Override binary/config paths only if explicitly provided
    if args.mnemonic_binary:
        cfg.mnemonic_binary = args.mnemonic_binary
    if args.mnemonic_config:
        cfg.mnemonic_config = args.mnemonic_config

    asyncio.run(run_session(cfg, initial_prompt=args.task))


if __name__ == "__main__":
    cli()
