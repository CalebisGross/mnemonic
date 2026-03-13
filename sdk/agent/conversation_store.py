"""Persistent conversation storage for the mnemonic-agent web chat.

Stores conversations as individual JSON files in evolution/conversations/.
Maintains a lightweight _index.json for fast listing.
"""
from __future__ import annotations

import json
import logging
import re
import threading
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

MAX_CONVERSATIONS = 50


class ConversationStore:
    """Manages conversation persistence in evolution/conversations/."""

    def __init__(self, evolution_dir: str) -> None:
        self._dir = Path(evolution_dir) / "conversations"
        self._dir.mkdir(parents=True, exist_ok=True)
        self._index_path = self._dir / "_index.json"
        self._prefs_path = self._dir / "_preferences.json"
        self._lock = threading.Lock()

    def _safe_conv_path(self, conv_id: str) -> Path | None:
        """Return the conversation file path, or None if the ID is invalid.

        Guards against path traversal by ensuring the resolved path stays
        inside self._dir and the ID matches the expected format.
        """
        if not re.fullmatch(r"conv-[0-9a-f]{8}", conv_id):
            logger.warning("Rejected invalid conversation ID: %r", conv_id)
            return None
        path = (self._dir / f"{conv_id}.json").resolve()
        if not str(path).startswith(str(self._dir.resolve())):
            logger.warning("Path traversal attempt blocked: %r", conv_id)
            return None
        return path

    # ── CRUD ──────────────────────────────────────────────────────────

    def new_conversation(self, model: str) -> dict[str, Any]:
        """Create a new conversation and return its metadata."""
        conv_id = f"conv-{uuid.uuid4().hex[:8]}"
        now = datetime.now(timezone.utc).isoformat()
        conv: dict[str, Any] = {
            "id": conv_id,
            "title": "New conversation",
            "created_at": now,
            "updated_at": now,
            "model": model,
            "message_count": 0,
            "total_cost_usd": 0.0,
            "messages": [],
        }
        with self._lock:
            self._save_conversation(conv)
            self._update_index(conv)
        return conv

    def append_message(self, conv_id: str, message: dict[str, Any]) -> None:
        """Append a message to a conversation.

        Auto-titles from the first user message (truncated to 60 chars).
        """
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return
            conv["messages"].append(message)
            conv["message_count"] = sum(
                1 for m in conv["messages"] if m.get("role") in ("user", "assistant")
            )
            conv["updated_at"] = datetime.now(timezone.utc).isoformat()
            # Auto-title from first user message
            if conv["title"] == "New conversation":
                for m in conv["messages"]:
                    if m.get("role") == "user":
                        title = m["content"].strip().replace("\n", " ")
                        conv["title"] = title[:60] + ("..." if len(title) > 60 else "")
                        break
            self._save_conversation(conv)
            self._update_index(conv)

    def update_cost(self, conv_id: str, cost_usd: float) -> None:
        """Add to the conversation's total cost."""
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return
            conv["total_cost_usd"] = round(
                conv.get("total_cost_usd", 0.0) + cost_usd, 6
            )
            conv["updated_at"] = datetime.now(timezone.utc).isoformat()
            self._save_conversation(conv)
            self._update_index(conv)

    def list_conversations(self) -> list[dict[str, Any]]:
        """Return conversation summaries sorted by updated_at desc."""
        with self._lock:
            index = self._load_index()
            return sorted(
                index.get("conversations", []),
                key=lambda c: c.get("updated_at", ""),
                reverse=True,
            )

    def get_conversation(self, conv_id: str) -> dict[str, Any] | None:
        """Load a full conversation including messages."""
        with self._lock:
            return self._load_conversation(conv_id)

    def delete_conversation(self, conv_id: str) -> bool:
        """Delete a conversation file and remove from index."""
        with self._lock:
            path = self._safe_conv_path(conv_id)
            if path is None:
                return False
            if path.exists():
                path.unlink()
            index = self._load_index()
            index["conversations"] = [
                c for c in index.get("conversations", []) if c["id"] != conv_id
            ]
            self._save_index(index)
        return True

    # ── Session resume ──────────────────────────────────────────────

    def set_session_id(self, conv_id: str, session_id: str) -> None:
        """Store the CLI session ID for a conversation.

        This links our conversation to the Claude CLI's transcript,
        enabling true conversation resume via ``--resume``.
        """
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return
            conv["session_id"] = session_id
            self._save_conversation(conv)

    def get_session_id(self, conv_id: str) -> str | None:
        """Retrieve the stored CLI session ID, or None for old conversations."""
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return None
            return conv.get("session_id")

    # ── Crash-safe streaming persistence ─────────────────────────

    def upsert_assistant_message(
        self, conv_id: str, content: str, timestamp: str,
    ) -> None:
        """Update the in-progress assistant message, or create one.

        Called during streaming to incrementally persist assistant text.
        Messages are marked ``"streaming": True`` until finalized.
        """
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return
            messages = conv.get("messages", [])
            # Update the existing streaming assistant message if present
            if (
                messages
                and messages[-1].get("role") == "assistant"
                and messages[-1].get("streaming")
            ):
                messages[-1]["content"] = content
                messages[-1]["timestamp"] = timestamp
            else:
                messages.append({
                    "role": "assistant",
                    "content": content,
                    "timestamp": timestamp,
                    "streaming": True,
                })
            conv["messages"] = messages
            conv["updated_at"] = timestamp
            self._save_conversation(conv)
            # Skip _update_index on every flush — too expensive during streaming

    def finalize_assistant_message(self, conv_id: str) -> None:
        """Remove the streaming flag and update the index."""
        with self._lock:
            conv = self._load_conversation(conv_id)
            if conv is None:
                return
            messages = conv.get("messages", [])
            if messages and messages[-1].get("role") == "assistant":
                messages[-1].pop("streaming", None)
                conv["messages"] = messages
                conv["message_count"] = sum(
                    1 for m in messages if m.get("role") in ("user", "assistant")
                )
                self._save_conversation(conv)
                self._update_index(conv)

    # ── Preferences ──────────────────────────────────────────────────

    def get_preferred_model(self) -> str | None:
        """Load the user's preferred model from _preferences.json."""
        try:
            if self._prefs_path.exists():
                prefs = json.loads(self._prefs_path.read_text())
                return prefs.get("model")
        except (json.JSONDecodeError, OSError):
            pass
        return None

    def set_preferred_model(self, model: str) -> None:
        """Persist the user's preferred model."""
        with self._lock:
            prefs: dict[str, Any] = {}
            try:
                if self._prefs_path.exists():
                    prefs = json.loads(self._prefs_path.read_text())
            except (json.JSONDecodeError, OSError):
                pass
            prefs["model"] = model
            try:
                self._prefs_path.write_text(json.dumps(prefs, indent=2))
            except OSError as e:
                logger.warning("Failed to save preferences: %s", e)

    # ── Global task counter ───────────────────────────────────────────

    def get_task_count(self) -> int:
        """Load the global task counter from preferences."""
        try:
            if self._prefs_path.exists():
                prefs = json.loads(self._prefs_path.read_text())
                return prefs.get("task_count", 0)
        except (json.JSONDecodeError, OSError):
            pass
        return 0

    def set_task_count(self, count: int) -> None:
        """Persist the global task counter."""
        with self._lock:
            prefs: dict[str, Any] = {}
            try:
                if self._prefs_path.exists():
                    prefs = json.loads(self._prefs_path.read_text())
            except (json.JSONDecodeError, OSError):
                pass
            prefs["task_count"] = count
            try:
                self._prefs_path.write_text(json.dumps(prefs, indent=2))
            except OSError as e:
                logger.warning("Failed to save task count: %s", e)

    # ── Internal helpers ─────────────────────────────────────────────

    def _load_conversation(self, conv_id: str) -> dict[str, Any] | None:
        path = self._safe_conv_path(conv_id)
        if path is None or not path.exists():
            return None
        try:
            return json.loads(path.read_text())
        except (json.JSONDecodeError, OSError):
            return None

    def _save_conversation(self, conv: dict[str, Any]) -> None:
        path = self._safe_conv_path(conv["id"])
        if path is None:
            return
        try:
            path.write_text(json.dumps(conv, indent=2))
        except OSError as e:
            logger.warning("Failed to save conversation %s: %s", conv["id"], e)

    def _load_index(self) -> dict[str, Any]:
        if not self._index_path.exists():
            return {"conversations": []}
        try:
            return json.loads(self._index_path.read_text())
        except (json.JSONDecodeError, OSError):
            return {"conversations": []}

    def _save_index(self, index: dict[str, Any]) -> None:
        # Rotation: keep only MAX_CONVERSATIONS
        convs = index.get("conversations", [])
        if len(convs) > MAX_CONVERSATIONS:
            convs.sort(key=lambda c: c.get("updated_at", ""), reverse=True)
            removed = convs[MAX_CONVERSATIONS:]
            convs = convs[:MAX_CONVERSATIONS]
            index["conversations"] = convs
            for old in removed:
                old_path = self._safe_conv_path(old.get("id", ""))
                if old_path is not None and old_path.exists():
                    try:
                        old_path.unlink()
                    except OSError:
                        pass
        try:
            self._index_path.write_text(json.dumps(index, indent=2))
        except OSError as e:
            logger.warning("Failed to save conversation index: %s", e)

    def _update_index(self, conv: dict[str, Any]) -> None:
        # Caller must hold self._lock
        index = self._load_index()
        entry: dict[str, Any] = {
            "id": conv["id"],
            "title": conv.get("title", "Untitled"),
            "created_at": conv.get("created_at", ""),
            "updated_at": conv.get("updated_at", ""),
            "message_count": conv.get("message_count", 0),
            "total_cost_usd": conv.get("total_cost_usd", 0.0),
            "preview": "",
        }
        for m in conv.get("messages", []):
            if m.get("role") == "user":
                entry["preview"] = m["content"][:100]
                break
        # Upsert
        found = False
        for i, c in enumerate(index.get("conversations", [])):
            if c["id"] == conv["id"]:
                index["conversations"][i] = entry
                found = True
                break
        if not found:
            index.setdefault("conversations", []).append(entry)
        self._save_index(index)
