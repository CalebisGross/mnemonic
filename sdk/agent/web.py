"""WebSocket server for the mnemonic-agent.

Wraps ClaudeSDKClient in a Starlette WebSocket endpoint so the agent
can be used from the Mnemonic web dashboard instead of only via terminal.

Supports persistent conversations (continue / delete / list), model
switching (with client recreation), and context injection for continuation.
"""
from __future__ import annotations

import argparse
import json
import logging
import os
import time
import uuid
from dataclasses import replace
from datetime import datetime, timezone

import uvicorn
from starlette.applications import Starlette
from starlette.routing import WebSocketRoute
from starlette.websockets import WebSocket, WebSocketDisconnect

from claude_agent_sdk import ClaudeSDKClient

from agent.config import Config
from agent.conversation_store import ConversationStore
from agent.options import build_options
from agent.prompts import POST_TASK_PROMPT, PRE_TASK_PROMPT
from agent.session import _record_task, stream_events

logger = logging.getLogger(__name__)


# ── Helpers ──────────────────────────────────────────────────────────

async def _send(ws: WebSocket, msg: dict) -> None:
    """Send a JSON message to the browser."""
    await ws.send_text(json.dumps(msg))


async def _stream_to_ws(
    client: ClaudeSDKClient,
    ws: WebSocket,
    store: ConversationStore | None = None,
    conv_id: str | None = None,
) -> tuple[float, int]:
    """Stream messages from the SDK client to the WebSocket.

    Uses the shared stream_events() generator from session.py.
    If *store* and *conv_id* are provided, assistant text and tool calls
    are also persisted to the conversation store.

    Returns (cost_usd, turns).
    """
    cost_usd = 0.0
    turns = 0
    accumulated_text = ""

    async for event_type, data in stream_events(client):
        if event_type == "text":
            await _send(ws, {"type": "text", "content": data})
            accumulated_text += data
        elif event_type == "thinking":
            if data:
                await _send(ws, {"type": "thinking", "summary": data})
        elif event_type == "tool_use":
            name, inp = data
            if len(inp) > 200:
                inp = inp[:197] + "..."
            await _send(ws, {
                "type": "tool_use",
                "name": name,
                "input": inp,
            })
            if store and conv_id:
                store.append_message(conv_id, {
                    "role": "tool_use",
                    "name": name,
                    "timestamp": datetime.now(timezone.utc).isoformat(),
                })
        elif event_type == "done":
            cost_usd, turns = data

    # Persist accumulated assistant text as a single message
    if store and conv_id and accumulated_text.strip():
        store.append_message(conv_id, {
            "role": "assistant",
            "content": accumulated_text,
            "timestamp": datetime.now(timezone.utc).isoformat(),
        })

    return cost_usd, turns


async def _handle_task(
    client: ClaudeSDKClient,
    cfg: Config,
    task: str,
    session_id: str,
    task_count: int,
    ws: WebSocket,
    store: ConversationStore | None = None,
    conv_id: str | None = None,
) -> None:
    """Run pre-task recall -> main query -> post-task reflection, streaming to WS."""
    task_start = time.monotonic()
    started_iso = datetime.now(timezone.utc).isoformat()
    total_cost = 0.0
    total_turns = 0

    # Pre-task recall
    if not cfg.no_reflect:
        await _send(ws, {"type": "status", "status": "recalling"})
        await client.query(PRE_TASK_PROMPT.format(task=task))
        cost, turns = await _stream_to_ws(client, ws)  # don't persist recall phase
        total_cost += cost
        total_turns += turns

    # Main task
    await _send(ws, {"type": "status", "status": "working"})
    await client.query(task)
    cost, turns = await _stream_to_ws(client, ws, store, conv_id)
    total_cost += cost
    total_turns += turns

    # Post-task reflection
    if not cfg.no_reflect:
        await _send(ws, {"type": "status", "status": "reflecting"})
        await client.query(POST_TASK_PROMPT)
        cost, turns = await _stream_to_ws(client, ws)  # don't persist reflection
        total_cost += cost
        total_turns += turns

    # Record telemetry
    evolved = (task_count + 1) % cfg.evolve_interval == 0
    duration_ms = int((time.monotonic() - task_start) * 1000)
    _record_task(
        evolution_dir=cfg.evolution_dir,
        session_id=session_id,
        model=cfg.model,
        description=task,
        started=started_iso,
        duration_ms=duration_ms,
        cost_usd=total_cost,
        turns=total_turns,
        evolved=evolved,
    )

    # Update conversation cost
    if store and conv_id:
        store.update_cost(conv_id, total_cost)

    await _send(ws, {
        "type": "done",
        "cost_usd": round(total_cost, 6),
        "turns": total_turns,
    })


# ── App factory ──────────────────────────────────────────────────────

def make_app(cfg: Config) -> Starlette:
    """Create the Starlette application with a WebSocket chat endpoint."""
    store = ConversationStore(cfg.evolution_dir)

    async def ws_endpoint(ws: WebSocket) -> None:
        await ws.accept()
        session_id = f"web-{uuid.uuid4().hex[:8]}"
        task_count = 0
        continuation_context: str | None = None
        current_conv_id: str | None = None
        # Per-connection model — read saved preference, fall back to cfg default
        session_model = store.get_preferred_model() or cfg.model

        logger.info("WebSocket connected, starting agent session %s", session_id)

        try:
            while True:  # Client lifecycle loop — restarts when model changes
                session_cfg = replace(cfg, model=session_model)
                options = build_options(session_cfg)
                async with ClaudeSDKClient(options=options) as client:
                    await _send(ws, {"type": "status", "status": "ready"})
                    await _send(ws, {
                        "type": "conversation_started",
                        "id": current_conv_id,
                        "model": session_model,
                    })

                    reconnect = False
                    while True:  # Message loop
                        try:
                            raw = await ws.receive_text()
                        except WebSocketDisconnect:
                            logger.info("WebSocket disconnected: session %s", session_id)
                            return

                        try:
                            msg = json.loads(raw)
                        except json.JSONDecodeError:
                            await _send(ws, {"type": "error", "message": "Invalid JSON"})
                            continue

                        msg_type = msg.get("type")

                        # ── Conversation management ──

                        if msg_type == "list_conversations":
                            convs = store.list_conversations()
                            await _send(ws, {
                                "type": "conversations_list",
                                "conversations": convs,
                            })

                        elif msg_type == "load_conversation":
                            cid = msg.get("id", "")
                            loaded = store.get_conversation(cid)
                            if loaded:
                                current_conv_id = cid
                                continuation_context = store.build_continuation_summary(cid)
                                await _send(ws, {
                                    "type": "conversation_loaded",
                                    "conversation": loaded,
                                })
                            else:
                                await _send(ws, {
                                    "type": "error",
                                    "message": "Conversation not found",
                                })

                        elif msg_type == "delete_conversation":
                            cid = msg.get("id", "")
                            store.delete_conversation(cid)
                            if cid == current_conv_id:
                                current_conv_id = None
                                continuation_context = None
                                await _send(ws, {
                                    "type": "conversation_started",
                                    "id": None,
                                    "model": session_model,
                                })
                            await _send(ws, {
                                "type": "conversation_deleted",
                                "id": cid,
                            })

                        elif msg_type == "new_conversation":
                            current_conv_id = None
                            continuation_context = None
                            await _send(ws, {
                                "type": "conversation_started",
                                "id": None,
                                "model": session_model,
                            })

                        # ── Model switching ──

                        elif msg_type == "set_model":
                            new_model = msg.get("model", "").strip()
                            if new_model:
                                session_model = new_model
                                store.set_preferred_model(new_model)
                                logger.info("Model changed to %s", new_model)
                                # Preserve conversation context across model switch
                                if current_conv_id:
                                    continuation_context = store.build_continuation_summary(
                                        current_conv_id
                                    )
                                await _send(ws, {
                                    "type": "model_set",
                                    "model": new_model,
                                })
                                reconnect = True
                                break  # Exit message loop to recreate client

                        # ── Chat message ──

                        elif msg_type == "message":
                            content = (msg.get("content") or "").strip()
                            if not content:
                                continue

                            # Lazy conversation creation — create on first message
                            if current_conv_id is None:
                                conv = store.new_conversation(session_model)
                                current_conv_id = conv["id"]
                                await _send(ws, {
                                    "type": "conversation_started",
                                    "id": conv["id"],
                                    "model": session_model,
                                })

                            # Persist user message
                            now = datetime.now(timezone.utc).isoformat()
                            store.append_message(current_conv_id, {
                                "role": "user",
                                "content": content,
                                "timestamp": now,
                            })

                            # On first message after loading a prior conversation,
                            # inject the continuation context summary.
                            effective_task = content
                            if continuation_context:
                                effective_task = (
                                    continuation_context
                                    + "\n\n## Current Message\n"
                                    + content
                                )
                                continuation_context = None  # only inject once

                            try:
                                await _handle_task(
                                    client, session_cfg, effective_task,
                                    session_id, task_count, ws,
                                    store, current_conv_id,
                                )
                                task_count += 1
                            except Exception as exc:
                                logger.exception("Error handling task: %s", exc)
                                await _send(ws, {"type": "error", "message": str(exc)})

                    if not reconnect:
                        break  # Clean exit from client lifecycle loop

        except WebSocketDisconnect:
            logger.info("WebSocket disconnected before session init: %s", session_id)
        except Exception as exc:
            logger.exception("Fatal error in WebSocket handler: %s", exc)
            try:
                await _send(ws, {"type": "error", "message": f"Server error: {exc}"})
                await ws.close()
            except Exception:
                pass

    return Starlette(routes=[WebSocketRoute("/ws", ws_endpoint)])


# ── CLI entry point ──────────────────────────────────────────────────

def main() -> None:
    # The bundled Claude CLI refuses to start if CLAUDECODE is set (nested session
    # detection). Unset it so the SDK subprocess can launch cleanly.
    os.environ.pop("CLAUDECODE", None)

    parser = argparse.ArgumentParser(
        prog="agent.web",
        description="Mnemonic Agent WebSocket server",
    )
    parser.add_argument("--port", type=int, default=9998)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--model", default=None)
    parser.add_argument("--mnemonic-binary", default=None)
    parser.add_argument("--mnemonic-config", default=None)
    parser.add_argument("--cwd", default=None)
    parser.add_argument("--permission-mode", default="acceptEdits")
    parser.add_argument("--evolve-interval", type=int, default=5)
    parser.add_argument("--no-reflect", action="store_true")
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args()

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    cfg = Config(
        permission_mode=args.permission_mode,
        evolve_interval=args.evolve_interval,
        no_reflect=args.no_reflect,
        verbose=args.verbose,
    )
    if args.model:
        cfg.model = args.model
    if args.mnemonic_binary:
        cfg.mnemonic_binary = args.mnemonic_binary
    if args.mnemonic_config:
        cfg.mnemonic_config = args.mnemonic_config
    if args.cwd:
        cfg.project_cwd = args.cwd

    app = make_app(cfg)
    logger.info("Starting agent WebSocket server on %s:%d", args.host, args.port)
    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
