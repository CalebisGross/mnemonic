"""Persistent conversation storage for the mnemonic-agent web chat.

Stores conversations as individual JSON files in evolution/conversations/.
Maintains a lightweight _index.json for fast listing.
"""
from __future__ import annotations

import json
import logging
import threading
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

MAX_CONVERSATIONS = 50
MAX_SUMMARY_CHARS = 4000
ASSISTANT_TRUNCATE = 500


class ConversationStore:
    """Manages conversation persistence in evolution/conversations/."""

    def __init__(self, evolution_dir: str) -> None:
        self._dir = Path(evolution_dir) / "conversations"
        self._dir.mkdir(parents=True, exist_ok=True)
        self._index_path = self._dir / "_index.json"
        self._prefs_path = self._dir / "_preferences.json"
        self._index_lock = threading.Lock()

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
        self._save_conversation(conv)
        self._update_index(conv)
        return conv

    def append_message(self, conv_id: str, message: dict[str, Any]) -> None:
        """Append a message to a conversation.

        Auto-titles from the first user message (truncated to 60 chars).
        """
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
        index = self._load_index()
        return sorted(
            index.get("conversations", []),
            key=lambda c: c.get("updated_at", ""),
            reverse=True,
        )

    def get_conversation(self, conv_id: str) -> dict[str, Any] | None:
        """Load a full conversation including messages."""
        return self._load_conversation(conv_id)

    def delete_conversation(self, conv_id: str) -> bool:
        """Delete a conversation file and remove from index."""
        path = self._dir / f"{conv_id}.json"
        if path.exists():
            path.unlink()
        with self._index_lock:
            index = self._load_index()
            index["conversations"] = [
                c for c in index.get("conversations", []) if c["id"] != conv_id
            ]
            self._save_index(index)
        return True

    # ── Continuation ─────────────────────────────────────────────────

    def build_continuation_summary(self, conv_id: str) -> str | None:
        """Build a condensed transcript of a prior conversation.

        Used to inject context when continuing a conversation with a fresh
        SDK subprocess. Truncates assistant messages and caps total length.
        """
        conv = self._load_conversation(conv_id)
        if conv is None:
            return None

        lines: list[str] = []
        lines.append(f"## Prior Conversation Context (id: {conv_id})")
        lines.append(f"Title: {conv.get('title', 'Untitled')}")
        lines.append(f"Date: {conv.get('created_at', 'unknown')}")
        lines.append("")

        total_chars = sum(len(ln) for ln in lines)

        for msg in conv.get("messages", []):
            role = msg.get("role", "")
            content = msg.get("content", "")

            if role == "user":
                line = f"User: {content}"
            elif role == "assistant":
                truncated = (
                    content[:ASSISTANT_TRUNCATE] + "..."
                    if len(content) > ASSISTANT_TRUNCATE
                    else content
                )
                line = f"Assistant: {truncated}"
            elif role == "tool_use":
                line = f"[Tool: {msg.get('name', '?')}]"
            else:
                continue

            if total_chars + len(line) > MAX_SUMMARY_CHARS:
                lines.append("... (earlier messages trimmed)")
                break
            lines.append(line)
            total_chars += len(line)

        lines.append("")
        lines.append("---")
        lines.append(
            "The user is continuing this conversation. "
            "Maintain context from the prior exchange above."
        )
        return "\n".join(lines)

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

    # ── Internal helpers ─────────────────────────────────────────────

    def _load_conversation(self, conv_id: str) -> dict[str, Any] | None:
        path = self._dir / f"{conv_id}.json"
        if not path.exists():
            return None
        try:
            return json.loads(path.read_text())
        except (json.JSONDecodeError, OSError):
            return None

    def _save_conversation(self, conv: dict[str, Any]) -> None:
        path = self._dir / f"{conv['id']}.json"
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
                old_path = self._dir / f"{old['id']}.json"
                if old_path.exists():
                    try:
                        old_path.unlink()
                    except OSError:
                        pass
        try:
            self._index_path.write_text(json.dumps(index, indent=2))
        except OSError as e:
            logger.warning("Failed to save conversation index: %s", e)

    def _update_index(self, conv: dict[str, Any]) -> None:
        with self._index_lock:
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
