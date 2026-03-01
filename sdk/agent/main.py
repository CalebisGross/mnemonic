from __future__ import annotations

import argparse
import asyncio
import logging
import os
import sys

from agent.config import (
    DEFAULT_EVOLVE_INTERVAL,
    DEFAULT_MODEL,
    DEFAULT_PERMISSION_MODE,
    DEFAULT_SUBAGENT_MODEL,
    Config,
)
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
        default=os.environ.get("MNEMONIC_AGENT_MODEL", DEFAULT_MODEL),
        help=f"Claude model to use (default: {DEFAULT_MODEL})",
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
        default=DEFAULT_PERMISSION_MODE,
        help=f"Tool permission mode (default: {DEFAULT_PERMISSION_MODE})",
    )
    parser.add_argument(
        "--evolve-interval",
        type=int,
        default=DEFAULT_EVOLVE_INTERVAL,
        help=f"Tasks between evolution cycles (default: {DEFAULT_EVOLVE_INTERVAL})",
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
        default=DEFAULT_SUBAGENT_MODEL,
        help=f"Model for subagents: code-reviewer, test-runner, memory-archivist (default: {DEFAULT_SUBAGENT_MODEL})",
    )
    parser.add_argument(
        "--evolution-dir",
        default=None,
        help="Path to evolution directory (default: sdk/evolution or $MNEMONIC_EVOLUTION_DIR)",
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
    if args.evolution_dir:
        cfg.evolution_dir_override = args.evolution_dir

    asyncio.run(run_session(cfg, initial_prompt=args.task))


if __name__ == "__main__":
    cli()
