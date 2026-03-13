from __future__ import annotations

import json
import logging
import time
import uuid
from collections.abc import AsyncGenerator, Callable
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

MAX_SESSIONS = 100

from claude_agent_sdk import (
    AssistantMessage,
    ClaudeSDKClient,
    ResultMessage,
    SystemMessage,
    TextBlock,
    ThinkingBlock,
    ToolResultBlock,
    ToolUseBlock,
    UserMessage,
)

from agent.config import Config
from agent.options import build_options
from agent.prompts import EVOLVE_PROMPT, POST_TASK_PROMPT, PRE_TASK_PROMPT


# --- Telemetry ---

@dataclass
class TaskMetrics:
    cost_usd: float = 0.0
    turns: int = 0
    session_id: str = ""


# --- Shared streaming ---

async def stream_events(
    client: ClaudeSDKClient,
) -> AsyncGenerator[tuple[str, Any], None]:
    """Yield (event_type, data) tuples from SDK client messages.

    Event types:
      ("text", str) — assistant text content
      ("thinking", str) — thinking summary (truncated to 300 chars)
      ("tool_use", (name, input_str, tool_use_id)) — tool invocation
      ("tool_result", (tool_use_id, content_str, is_error)) — tool result
      ("system", (subtype, data_str)) — system message
      ("done", (cost_usd, turns)) — final result metrics
    """
    async for msg in client.receive_messages():
        if isinstance(msg, AssistantMessage):
            for block in msg.content:
                if isinstance(block, TextBlock):
                    yield ("text", block.text)
                elif isinstance(block, ThinkingBlock):
                    yield ("thinking", (block.thinking or "")[:300])
                elif isinstance(block, ToolUseBlock):
                    yield ("tool_use", (block.name, str(block.input), block.id))
        elif isinstance(msg, UserMessage):
            # Tool results come as UserMessage with ToolResultBlock content
            if isinstance(msg.content, list):
                for block in msg.content:
                    if isinstance(block, ToolResultBlock):
                        content_str = ""
                        if isinstance(block.content, str):
                            content_str = block.content
                        elif isinstance(block.content, list):
                            # Extract text from content blocks
                            parts = []
                            for item in block.content:
                                if isinstance(item, dict) and item.get("type") == "text":
                                    parts.append(item.get("text", ""))
                            content_str = "\n".join(parts)
                        yield ("tool_result", (
                            block.tool_use_id,
                            content_str,
                            bool(block.is_error),
                        ))
        elif isinstance(msg, SystemMessage):
            yield ("system", (msg.subtype, str(msg.data)))
        elif isinstance(msg, ResultMessage):
            cost = getattr(msg, "total_cost_usd", 0.0) or 0.0
            turns = getattr(msg, "num_turns", 0) or 0
            sid = getattr(msg, "session_id", "") or ""
            yield ("done", (cost, turns, sid))
            return


async def _stream_responses(
    client: ClaudeSDKClient,
    verbose: bool = False,
) -> TaskMetrics:
    """Stream and print messages from the client. Returns metrics from ResultMessage."""
    metrics = TaskMetrics()
    async for event_type, data in stream_events(client):
        if event_type == "text":
            print(data, flush=True)
        elif event_type == "thinking" and verbose:
            logger.debug("[thinking] %s...", data[:200])
        elif event_type == "tool_use" and verbose:
            name, inp, _tid = data
            logger.debug("[tool] %s(%s)", name, _truncate(inp, 120))
        elif event_type == "system" and verbose:
            subtype, d = data
            logger.debug("[system] %s: %s", subtype, _truncate(d, 200))
        elif event_type == "done":
            metrics.cost_usd, metrics.turns, metrics.session_id = data
            if verbose:
                parts = []
                if metrics.turns:
                    parts.append(f"turns={metrics.turns}")
                if metrics.cost_usd:
                    parts.append(f"cost=${metrics.cost_usd:.4f}")
                if parts:
                    logger.debug("[done] %s", ", ".join(parts))
    return metrics


def _truncate(s: str, max_len: int) -> str:
    return s if len(s) <= max_len else s[: max_len - 3] + "..."


def _record_task(
    evolution_dir: str,
    session_id: str,
    model: str,
    description: str,
    started: str,
    duration_ms: int,
    cost_usd: float,
    turns: int,
    evolved: bool,
    conv_id: str | None = None,
) -> None:
    """Append a task record to sessions.json."""
    sessions_path = Path(evolution_dir) / "sessions.json"

    # Load existing data
    data: dict = {"sessions": []}
    if sessions_path.exists():
        try:
            data = json.loads(sessions_path.read_text())
        except (json.JSONDecodeError, OSError):
            data = {"sessions": []}

    # Find or create current session
    current_session = None
    for s in data["sessions"]:
        if s.get("id") == session_id:
            current_session = s
            break

    if current_session is None:
        current_session = {
            "id": session_id,
            "started": started,
            "model": model,
            "tasks": [],
        }
        data["sessions"].append(current_session)

    # Append task
    task_entry: dict = {
        "description": description[:200],
        "started": started,
        "duration_ms": duration_ms,
        "cost_usd": round(cost_usd, 6),
        "turns": turns,
        "evolved": evolved,
    }
    if conv_id:
        task_entry["conv_id"] = conv_id
    current_session["tasks"].append(task_entry)

    # Rotate: keep only the most recent MAX_SESSIONS sessions
    if len(data["sessions"]) > MAX_SESSIONS:
        data["sessions"] = data["sessions"][-MAX_SESSIONS:]

    # Write back
    try:
        sessions_path.write_text(json.dumps(data, indent=2))
    except OSError as e:
        logger.warning("Failed to write sessions.json: %s", e)  # Non-critical — don't crash the agent


async def run_session(cfg: Config, initial_prompt: str | None = None) -> None:
    """Main REPL loop with pre/post task orchestration."""
    options = build_options(cfg)
    task_count = 0
    session_id = f"session-{uuid.uuid4().hex[:8]}"

    async with ClaudeSDKClient(options=options) as client:
        # If an initial task was provided via CLI, run it
        if initial_prompt:
            await _run_task(client, cfg, initial_prompt, session_id, task_count)
            task_count += 1
            if task_count % cfg.evolve_interval == 0:
                await _run_evolution(client, cfg)
            return

        # Interactive REPL
        print("mnemonic-agent ready. Type your task, or 'exit' to quit.\n", flush=True)
        while True:
            try:
                user_input = input("> ").strip()
            except (EOFError, KeyboardInterrupt):
                print("\nGoodbye.", flush=True)
                break

            if not user_input:
                continue
            if user_input.lower() in ("/exit", "/quit", "exit", "quit"):
                break

            await _run_task(client, cfg, user_input, session_id, task_count)
            task_count += 1

            # Periodic evolution cycle
            if task_count % cfg.evolve_interval == 0:
                await _run_evolution(client, cfg)


async def _orchestrate_task(
    client: ClaudeSDKClient,
    cfg: Config,
    task: str,
    session_id: str,
    task_count: int,
    main_stream_fn: Callable,
    side_stream_fn: Callable | None = None,
    on_phase: Callable | None = None,
    conv_id: str | None = None,
) -> TaskMetrics:
    """Shared task orchestration: pre-task recall → main → post-task reflect → telemetry.

    Args:
        main_stream_fn: async (client) -> TaskMetrics — streams the main task output.
        side_stream_fn: async (client) -> TaskMetrics — streams pre/post phases.
            Defaults to main_stream_fn if not provided.
        on_phase: optional async (phase_name: str) -> None — called before each phase
            with "recalling", "working", or "reflecting".

    Returns accumulated TaskMetrics for the full orchestration.
    """
    if side_stream_fn is None:
        side_stream_fn = main_stream_fn

    task_start = time.monotonic()
    started_iso = datetime.now(timezone.utc).isoformat()
    total = TaskMetrics()

    # === PRE-TASK: automatic context recall ===
    if not cfg.no_reflect:
        if on_phase:
            await on_phase("recalling")
        await client.query(PRE_TASK_PROMPT.replace("{task}", task))
        m = await side_stream_fn(client)
        total.cost_usd += m.cost_usd
        total.turns += m.turns
        if m.session_id:
            total.session_id = m.session_id

    # === MAIN TASK ===
    if on_phase:
        await on_phase("working")
    await client.query(task)
    m = await main_stream_fn(client)
    total.cost_usd += m.cost_usd
    total.turns += m.turns
    if m.session_id:
        total.session_id = m.session_id

    # === POST-TASK: automatic reflection ===
    if not cfg.no_reflect:
        if on_phase:
            await on_phase("reflecting")
        await client.query(POST_TASK_PROMPT)
        m = await side_stream_fn(client)
        total.cost_usd += m.cost_usd
        total.turns += m.turns
        if m.session_id:
            total.session_id = m.session_id

    # === Record telemetry ===
    evolved = (task_count + 1) % cfg.evolve_interval == 0
    duration_ms = int((time.monotonic() - task_start) * 1000)
    _record_task(
        evolution_dir=cfg.evolution_dir,
        session_id=session_id,
        model=cfg.model,
        description=task,
        started=started_iso,
        duration_ms=duration_ms,
        cost_usd=total.cost_usd,
        turns=total.turns,
        evolved=evolved,
        conv_id=conv_id,
    )
    return total


async def _run_task(
    client: ClaudeSDKClient,
    cfg: Config,
    task: str,
    session_id: str,
    task_count: int,
) -> None:
    """CLI task execution — streams to stdout."""

    async def _cli_stream(c: ClaudeSDKClient) -> TaskMetrics:
        return await _stream_responses(c, verbose=cfg.verbose)

    async def _on_phase(phase: str) -> None:
        if cfg.verbose:
            logger.debug("\n[%s]%s", phase, f" {task}" if phase == "working" else "")

    await _orchestrate_task(
        client, cfg, task, session_id, task_count,
        main_stream_fn=_cli_stream,
        on_phase=_on_phase,
    )


async def _run_evolution(
    client: ClaudeSDKClient,
    cfg: Config,
) -> None:
    """Run a self-improvement cycle."""
    if cfg.no_reflect:
        return
    logger.info("\n[evolution] Running self-improvement cycle...")
    await client.query(EVOLVE_PROMPT)
    await _stream_responses(client, verbose=cfg.verbose)
    logger.info("[evolution] Complete.")
