"""WebSocket server for the mnemonic-agent.

Wraps ClaudeSDKClient in a Starlette WebSocket endpoint so the agent
can be used from the Mnemonic web dashboard instead of only via terminal.

Supports persistent conversations (continue / delete / list), model
switching (with client recreation), and session resume for continuation.
"""
from __future__ import annotations

import argparse
import asyncio
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

from claude_agent_sdk import CLINotFoundError, ClaudeSDKClient, ProcessError

from agent.config import DEFAULT_EVOLVE_INTERVAL, DEFAULT_PERMISSION_MODE, Config
from agent.conversation_store import ConversationStore
from agent.options import build_options
from agent.session import TaskMetrics, _orchestrate_task, _run_evolution, stream_events

logger = logging.getLogger(__name__)

WS_IDLE_TIMEOUT = 300  # seconds

_AUTH_KEYWORDS = (
    "not logged in", "login", "auth", "oauth",
    "unauthorized", "credential", "not authenticated",
)


def _is_auth_error(exc: ProcessError) -> bool:
    """Return True if a ProcessError looks like a Claude CLI authentication failure."""
    text = " ".join(filter(None, [exc.stderr, str(exc)])).lower()
    return any(kw in text for kw in _AUTH_KEYWORDS)


# ── Helpers ──────────────────────────────────────────────────────────

async def _send(ws: WebSocket, msg: dict) -> None:
    """Send a JSON message to the browser."""
    await ws.send_text(json.dumps(msg))


async def _stream_to_ws(
    client: ClaudeSDKClient,
    ws: WebSocket,
    store: ConversationStore | None = None,
    conv_id: str | None = None,
) -> TaskMetrics:
    """Stream messages from the SDK client to the WebSocket.

    Uses the shared stream_events() generator from session.py.
    If *store* and *conv_id* are provided, assistant text and tool calls
    are persisted incrementally (crash-safe) to the conversation store.

    Returns TaskMetrics with cost, turns, and session_id.
    """
    metrics = TaskMetrics()
    # Track the latest text for the current assistant turn.
    # SDK sends full text (not deltas) for each AssistantMessage,
    # so we replace rather than accumulate.
    current_turn_text = ""
    # Throttle disk writes during streaming — save at most every 3 seconds.
    _last_flush = 0.0

    try:
        async for event_type, data in stream_events(client):
            if event_type == "text":
                await _send(ws, {"type": "text", "content": data})
                # Replace — SDK text blocks contain the full turn text
                current_turn_text = data
                # Throttled persistence: flush to disk at most every 3s
                now = time.monotonic()
                if store and conv_id and now - _last_flush >= 3.0:
                    store.upsert_assistant_message(
                        conv_id, current_turn_text,
                        datetime.now(timezone.utc).isoformat(),
                    )
                    _last_flush = now
            elif event_type == "thinking":
                if data:
                    await _send(ws, {"type": "thinking", "summary": data})
            elif event_type == "tool_use":
                name, inp, tool_use_id = data
                # Finalize current assistant text before tool_use
                if store and conv_id and current_turn_text.strip():
                    store.upsert_assistant_message(
                        conv_id, current_turn_text,
                        datetime.now(timezone.utc).isoformat(),
                    )
                    store.finalize_assistant_message(conv_id)
                    current_turn_text = ""
                # Truncate for WebSocket display, store full input
                ws_inp = inp[:797] + "..." if len(inp) > 800 else inp
                await _send(ws, {
                    "type": "tool_use",
                    "name": name,
                    "input": ws_inp,
                    "tool_use_id": tool_use_id,
                })
                if store and conv_id:
                    store.append_message(conv_id, {
                        "role": "tool_use",
                        "name": name,
                        "input": inp,
                        "timestamp": datetime.now(timezone.utc).isoformat(),
                    })
            elif event_type == "tool_result":
                tool_use_id, content, is_error = data
                # Truncate for WebSocket display
                ws_content = content[:2000] + "..." if len(content) > 2000 else content
                await _send(ws, {
                    "type": "tool_result",
                    "tool_use_id": tool_use_id,
                    "content": ws_content,
                    "is_error": is_error,
                })
            elif event_type == "done":
                metrics.cost_usd, metrics.turns, metrics.session_id = data
    finally:
        # Always persist whatever text we have — even on crash
        if store and conv_id and current_turn_text.strip():
            store.upsert_assistant_message(
                conv_id, current_turn_text,
                datetime.now(timezone.utc).isoformat(),
            )
            store.finalize_assistant_message(conv_id)

    return metrics


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

    async def _ws_main_stream(c: ClaudeSDKClient) -> TaskMetrics:
        return await _stream_to_ws(c, ws, store, conv_id)

    async def _ws_side_stream(c: ClaudeSDKClient) -> TaskMetrics:
        return await _stream_to_ws(c, ws)  # don't persist recall/reflect

    async def _on_phase(phase: str) -> None:
        await _send(ws, {"type": "status", "status": phase})

    total = await _orchestrate_task(
        client, cfg, task, session_id, task_count,
        main_stream_fn=_ws_main_stream,
        side_stream_fn=_ws_side_stream,
        on_phase=_on_phase,
    )

    # Update conversation cost and session ID
    if store and conv_id:
        store.update_cost(conv_id, total.cost_usd)
        if total.session_id:
            store.set_session_id(conv_id, total.session_id)

    await _send(ws, {
        "type": "done",
        "cost_usd": round(total.cost_usd, 6),
        "turns": total.turns,
    })


# ── WebSocket session ────────────────────────────────────────────────

class WebSocketSession:
    """Manages a single WebSocket connection's lifecycle and state."""

    def __init__(self, ws: WebSocket, cfg: Config, store: ConversationStore) -> None:
        self._ws = ws
        self._cfg = cfg
        self._store = store
        self._session_id = f"web-{uuid.uuid4().hex[:8]}"
        self._task_count = store.get_task_count()
        self._resume_session_id: str | None = None
        self._current_conv_id: str | None = None
        self._session_model = store.get_preferred_model() or cfg.model

    async def run(self) -> None:
        """Outer lifecycle — handles client recreation on model switch."""
        logger.info("WebSocket connected, starting agent session %s", self._session_id)
        try:
            while True:
                reconnect = await self._run_client_lifecycle()
                if not reconnect:
                    break
        except WebSocketDisconnect:
            logger.info("WebSocket disconnected: session %s", self._session_id)
        except CLINotFoundError as exc:
            logger.error("Claude CLI not found in session %s: %s", self._session_id, exc)
            try:
                await _send(self._ws, {
                    "type": "error",
                    "message": (
                        "Claude CLI not found. Install it with:\n\n"
                        "```\nnpm install -g @anthropic-ai/claude-code\n```"
                    ),
                })
                await self._ws.close()
            except Exception:
                pass
        except ProcessError as exc:
            if _is_auth_error(exc):
                logger.error("Claude CLI auth error in session %s: %s", self._session_id, exc)
                try:
                    await _send(self._ws, {
                        "type": "error",
                        "message": (
                            "Claude CLI is not authenticated. "
                            "Run `claude` in your terminal to log in, then reload this page."
                        ),
                    })
                    await self._ws.close()
                except Exception:
                    pass
            else:
                logger.exception("Fatal ProcessError in WebSocket handler: %s", self._session_id)
                try:
                    await _send(self._ws, {
                        "type": "error",
                        "message": "Internal server error — session terminated.",
                    })
                    await self._ws.close()
                except Exception:
                    pass
        except Exception:
            logger.exception("Fatal error in WebSocket handler: %s", self._session_id)
            try:
                await _send(self._ws, {
                    "type": "error",
                    "message": "Internal server error — session terminated.",
                })
                await self._ws.close()
            except Exception:
                pass

    async def _run_client_lifecycle(self) -> bool:
        """Create a ClaudeSDKClient and run the message loop.

        Returns True if the client should be recreated (model switch).
        If ``_resume_session_id`` is set, the CLI subprocess resumes from
        the prior session transcript, restoring full conversation history.
        """
        resume_id = self._resume_session_id
        self._resume_session_id = None  # consume once
        session_cfg = replace(
            self._cfg, model=self._session_model, resume=resume_id,
        )
        options = build_options(session_cfg)
        async with ClaudeSDKClient(options=options) as client:
            await _send(self._ws, {"type": "status", "status": "ready"})
            await _send(self._ws, {
                "type": "conversation_started",
                "id": self._current_conv_id,
                "model": self._session_model,
            })
            return await self._run_message_loop(client, session_cfg)

    async def _run_message_loop(
        self, client: ClaudeSDKClient, session_cfg: Config,
    ) -> bool:
        """Receive and dispatch messages. Returns True to trigger reconnect."""
        while True:
            try:
                raw = await asyncio.wait_for(
                    self._ws.receive_text(), timeout=WS_IDLE_TIMEOUT,
                )
            except asyncio.TimeoutError:
                await _send(self._ws, {
                    "type": "error",
                    "message": "Connection timed out due to inactivity.",
                })
                await self._ws.close()
                return False
            except WebSocketDisconnect:
                return False

            try:
                msg = json.loads(raw)
            except json.JSONDecodeError:
                await _send(self._ws, {"type": "error", "message": "Invalid JSON"})
                continue

            reconnect = await self._handle_message(msg, client, session_cfg)
            if reconnect:
                return True

        return False  # unreachable, but satisfies type checkers

    async def _handle_message(
        self,
        msg: dict,
        client: ClaudeSDKClient,
        session_cfg: Config,
    ) -> bool:
        """Dispatch a single message. Returns True to trigger reconnect."""
        msg_type = msg.get("type")

        if msg_type == "list_conversations":
            await self._handle_list_conversations()
        elif msg_type == "load_conversation":
            return await self._handle_load_conversation(msg)
        elif msg_type == "delete_conversation":
            await self._handle_delete_conversation(msg)
        elif msg_type == "new_conversation":
            return await self._handle_new_conversation()
        elif msg_type == "set_model":
            return await self._handle_set_model(msg)
        elif msg_type == "message":
            await self._handle_chat_message(msg, client, session_cfg)

        return False

    # ── Conversation management ──

    async def _handle_list_conversations(self) -> None:
        convs = self._store.list_conversations()
        await _send(self._ws, {
            "type": "conversations_list",
            "conversations": convs,
        })

    async def _handle_load_conversation(self, msg: dict) -> bool:
        """Load a conversation. Returns True to trigger client reconnect for resume."""
        cid = msg.get("id", "")
        loaded = self._store.get_conversation(cid)
        if loaded:
            self._current_conv_id = cid
            self._resume_session_id = self._store.get_session_id(cid)
            await _send(self._ws, {
                "type": "conversation_loaded",
                "conversation": loaded,
            })
            # Reconnect to resume the CLI session with full history
            if self._resume_session_id:
                return True
        else:
            await _send(self._ws, {
                "type": "error",
                "message": "Conversation not found",
            })
        return False

    async def _handle_delete_conversation(self, msg: dict) -> None:
        cid = msg.get("id", "")
        self._store.delete_conversation(cid)
        if cid == self._current_conv_id:
            self._current_conv_id = None
            self._resume_session_id = None
            await _send(self._ws, {
                "type": "conversation_started",
                "id": None,
                "model": self._session_model,
            })
        await _send(self._ws, {
            "type": "conversation_deleted",
            "id": cid,
        })

    async def _handle_new_conversation(self) -> bool:
        """Returns True to trigger client reconnect with a fresh CLI session."""
        self._current_conv_id = None
        self._resume_session_id = None  # no resume — fresh session
        await _send(self._ws, {
            "type": "conversation_started",
            "id": None,
            "model": self._session_model,
        })
        return True

    # ── Model switching ──

    async def _handle_set_model(self, msg: dict) -> bool:
        """Returns True to trigger client reconnect."""
        new_model = msg.get("model", "").strip()
        if not new_model:
            return False
        self._session_model = new_model
        self._store.set_preferred_model(new_model)
        logger.info("Model changed to %s", new_model)
        if self._current_conv_id:
            self._resume_session_id = self._store.get_session_id(
                self._current_conv_id
            )
        await _send(self._ws, {"type": "model_set", "model": new_model})
        return True

    # ── Chat ──

    async def _handle_chat_message(
        self, msg: dict, client: ClaudeSDKClient, session_cfg: Config,
    ) -> None:
        content = (msg.get("content") or "").strip()
        if not content:
            return

        # Lazy conversation creation — create on first message
        if self._current_conv_id is None:
            conv = self._store.new_conversation(self._session_model)
            self._current_conv_id = conv["id"]
            await _send(self._ws, {
                "type": "conversation_started",
                "id": conv["id"],
                "model": self._session_model,
            })

        # Persist user message
        now = datetime.now(timezone.utc).isoformat()
        self._store.append_message(self._current_conv_id, {
            "role": "user",
            "content": content,
            "timestamp": now,
        })

        try:
            await _handle_task(
                client, session_cfg, content,
                self._session_id, self._task_count, self._ws,
                self._store, self._current_conv_id,
            )
            self._task_count += 1
            self._store.set_task_count(self._task_count)
            # Periodic evolution cycle
            if self._task_count % session_cfg.evolve_interval == 0:
                await _send(self._ws, {"type": "status", "status": "evolving"})
                await _run_evolution(client, session_cfg)
        except Exception as exc:
            logger.exception("Error handling task: %s", exc)
            await _send(self._ws, {
                "type": "error",
                "message": "Task failed — check server logs for details.",
            })


# ── App factory ──────────────────────────────────────────────────────

def make_app(cfg: Config) -> Starlette:
    """Create the Starlette application with a WebSocket chat endpoint."""
    store = ConversationStore(cfg.evolution_dir)

    async def ws_endpoint(ws: WebSocket) -> None:
        await ws.accept()
        await WebSocketSession(ws, cfg, store).run()

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
    parser.add_argument("--permission-mode", default=DEFAULT_PERMISSION_MODE)
    parser.add_argument("--evolve-interval", type=int, default=DEFAULT_EVOLVE_INTERVAL)
    parser.add_argument("--no-reflect", action="store_true")
    parser.add_argument("--verbose", action="store_true")
    parser.add_argument("--evolution-dir", default=None,
                        help="Path to evolution directory (default: sdk/evolution or $MNEMONIC_EVOLUTION_DIR)")
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
    if args.evolution_dir:
        cfg.evolution_dir_override = args.evolution_dir

    app = make_app(cfg)
    logger.info("Starting agent WebSocket server on %s:%d", args.host, args.port)
    uvicorn.run(app, host=args.host, port=args.port, log_level="info")


if __name__ == "__main__":
    main()
