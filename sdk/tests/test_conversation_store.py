"""Tests for ConversationStore."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from agent.conversation_store import (
    MAX_CONVERSATIONS,
    ConversationStore,
)


class TestConversationStore(unittest.TestCase):
    def setUp(self):
        self._tmpdir = tempfile.TemporaryDirectory()
        self.tmpdir = self._tmpdir.name
        self.store = ConversationStore(self.tmpdir)

    def tearDown(self):
        self._tmpdir.cleanup()

    # ── new_conversation ─────────────────────────────────────────────

    def test_new_conversation_creates_file_and_index(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.assertIn("id", conv)
        self.assertTrue(conv["id"].startswith("conv-"))
        self.assertEqual(conv["model"], "claude-sonnet-4-6")
        self.assertEqual(conv["message_count"], 0)
        self.assertEqual(conv["messages"], [])
        # File exists
        path = Path(self.tmpdir) / "conversations" / f"{conv['id']}.json"
        self.assertTrue(path.exists())
        # Index has entry
        convs = self.store.list_conversations()
        self.assertEqual(len(convs), 1)
        self.assertEqual(convs[0]["id"], conv["id"])

    def test_new_conversation_default_title(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.assertEqual(conv["title"], "New conversation")

    # ── append_message ───────────────────────────────────────────────

    def test_append_message_adds_to_list(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.store.append_message(conv["id"], {
            "role": "user",
            "content": "Hello",
        })
        loaded = self.store.get_conversation(conv["id"])
        self.assertEqual(len(loaded["messages"]), 1)
        self.assertEqual(loaded["messages"][0]["content"], "Hello")

    def test_append_message_auto_titles_from_first_user(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.store.append_message(conv["id"], {
            "role": "user",
            "content": "Fix the authentication bug in login.py",
        })
        loaded = self.store.get_conversation(conv["id"])
        self.assertEqual(loaded["title"], "Fix the authentication bug in login.py")

    def test_append_message_truncates_long_title(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        long_msg = "x" * 100
        self.store.append_message(conv["id"], {
            "role": "user",
            "content": long_msg,
        })
        loaded = self.store.get_conversation(conv["id"])
        self.assertEqual(len(loaded["title"]), 63)  # 60 + "..."
        self.assertTrue(loaded["title"].endswith("..."))

    def test_append_message_counts_user_and_assistant(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.store.append_message(conv["id"], {"role": "user", "content": "hi"})
        self.store.append_message(conv["id"], {"role": "assistant", "content": "hello"})
        self.store.append_message(conv["id"], {"role": "tool_use", "name": "Read"})
        loaded = self.store.get_conversation(conv["id"])
        # tool_use should not count toward message_count
        self.assertEqual(loaded["message_count"], 2)

    def test_append_message_missing_conv_does_nothing(self):
        # Should not raise
        self.store.append_message("nonexistent", {"role": "user", "content": "hi"})

    # ── update_cost ──────────────────────────────────────────────────

    def test_update_cost_accumulates(self):
        conv = self.store.new_conversation("claude-sonnet-4-6")
        self.store.update_cost(conv["id"], 0.001)
        self.store.update_cost(conv["id"], 0.002)
        loaded = self.store.get_conversation(conv["id"])
        self.assertAlmostEqual(loaded["total_cost_usd"], 0.003, places=6)

    def test_update_cost_missing_conv_does_nothing(self):
        self.store.update_cost("nonexistent", 1.0)

    # ── list_conversations ───────────────────────────────────────────

    def test_list_conversations_sorted_by_updated_at_desc(self):
        c1 = self.store.new_conversation("m1")
        c2 = self.store.new_conversation("m2")
        # Touch c1 so it's updated more recently
        self.store.append_message(c1["id"], {"role": "user", "content": "later"})
        convs = self.store.list_conversations()
        self.assertEqual(len(convs), 2)
        self.assertEqual(convs[0]["id"], c1["id"])  # most recent first

    def test_list_conversations_empty(self):
        self.assertEqual(self.store.list_conversations(), [])

    # ── get_conversation ─────────────────────────────────────────────

    def test_get_conversation_returns_none_for_missing(self):
        self.assertIsNone(self.store.get_conversation("no-such-id"))

    def test_get_conversation_recovers_from_malformed_json(self):
        conv_dir = Path(self.tmpdir) / "conversations"
        (conv_dir / "bad-conv.json").write_text("not json{{{")
        self.assertIsNone(self.store.get_conversation("bad-conv"))

    # ── delete_conversation ──────────────────────────────────────────

    def test_delete_conversation_removes_file_and_index(self):
        conv = self.store.new_conversation("m")
        self.store.delete_conversation(conv["id"])
        self.assertIsNone(self.store.get_conversation(conv["id"]))
        self.assertEqual(len(self.store.list_conversations()), 0)

    def test_delete_nonexistent_does_not_raise(self):
        self.store.delete_conversation("no-such-id")

    # ── session_id ─────────────────────────────────────────────────

    def test_set_and_get_session_id(self):
        conv = self.store.new_conversation("m")
        self.store.set_session_id(conv["id"], "sess-abc123")
        self.assertEqual(self.store.get_session_id(conv["id"]), "sess-abc123")

    def test_get_session_id_returns_none_for_missing_conv(self):
        self.assertIsNone(self.store.get_session_id("no-such"))

    def test_get_session_id_returns_none_for_old_conv(self):
        """Old conversations without session_id should return None."""
        conv = self.store.new_conversation("m")
        self.assertIsNone(self.store.get_session_id(conv["id"]))

    def test_set_session_id_overwrites(self):
        conv = self.store.new_conversation("m")
        self.store.set_session_id(conv["id"], "sess-1")
        self.store.set_session_id(conv["id"], "sess-2")
        self.assertEqual(self.store.get_session_id(conv["id"]), "sess-2")

    # ── upsert_assistant_message ──────────────────────────────────

    def test_upsert_creates_new_streaming_message(self):
        conv = self.store.new_conversation("m")
        self.store.upsert_assistant_message(conv["id"], "Hello", "2026-01-01T00:00:00Z")
        loaded = self.store.get_conversation(conv["id"])
        self.assertEqual(len(loaded["messages"]), 1)
        msg = loaded["messages"][0]
        self.assertEqual(msg["role"], "assistant")
        self.assertEqual(msg["content"], "Hello")
        self.assertTrue(msg["streaming"])

    def test_upsert_updates_existing_streaming_message(self):
        conv = self.store.new_conversation("m")
        self.store.upsert_assistant_message(conv["id"], "Hello", "2026-01-01T00:00:00Z")
        self.store.upsert_assistant_message(conv["id"], "Hello world", "2026-01-01T00:00:01Z")
        loaded = self.store.get_conversation(conv["id"])
        # Should still be one message, updated in place
        self.assertEqual(len(loaded["messages"]), 1)
        self.assertEqual(loaded["messages"][0]["content"], "Hello world")

    def test_upsert_does_not_touch_finalized_message(self):
        conv = self.store.new_conversation("m")
        self.store.append_message(conv["id"], {
            "role": "assistant", "content": "finalized",
        })
        # New upsert should create a second message, not modify the first
        self.store.upsert_assistant_message(conv["id"], "new text", "2026-01-01T00:00:00Z")
        loaded = self.store.get_conversation(conv["id"])
        self.assertEqual(len(loaded["messages"]), 2)
        self.assertEqual(loaded["messages"][0]["content"], "finalized")
        self.assertEqual(loaded["messages"][1]["content"], "new text")

    def test_upsert_missing_conv_does_nothing(self):
        # Should not raise
        self.store.upsert_assistant_message("no-such", "text", "2026-01-01T00:00:00Z")

    # ── finalize_assistant_message ────────────────────────────────

    def test_finalize_removes_streaming_flag(self):
        conv = self.store.new_conversation("m")
        self.store.upsert_assistant_message(conv["id"], "Done", "2026-01-01T00:00:00Z")
        self.store.finalize_assistant_message(conv["id"])
        loaded = self.store.get_conversation(conv["id"])
        msg = loaded["messages"][0]
        self.assertNotIn("streaming", msg)
        # message_count should be updated (assistant counts)
        self.assertEqual(loaded["message_count"], 1)

    def test_finalize_updates_index(self):
        conv = self.store.new_conversation("m")
        self.store.append_message(conv["id"], {"role": "user", "content": "hi"})
        self.store.upsert_assistant_message(conv["id"], "response", "2026-01-01T00:00:00Z")
        self.store.finalize_assistant_message(conv["id"])
        convs = self.store.list_conversations()
        self.assertEqual(convs[0]["message_count"], 2)

    def test_finalize_missing_conv_does_nothing(self):
        self.store.finalize_assistant_message("no-such")

    # ── preferences ──────────────────────────────────────────────────

    def test_get_preferred_model_default_none(self):
        self.assertIsNone(self.store.get_preferred_model())

    def test_set_and_get_preferred_model(self):
        self.store.set_preferred_model("claude-opus-4-6")
        self.assertEqual(self.store.get_preferred_model(), "claude-opus-4-6")

    def test_set_preferred_model_overwrites(self):
        self.store.set_preferred_model("claude-sonnet-4-6")
        self.store.set_preferred_model("claude-opus-4-6")
        self.assertEqual(self.store.get_preferred_model(), "claude-opus-4-6")

    def test_get_preferred_model_recovers_from_malformed(self):
        prefs_path = Path(self.tmpdir) / "conversations" / "_preferences.json"
        prefs_path.write_text("bad json{")
        self.assertIsNone(self.store.get_preferred_model())

    # ── task counter ────────────────────────────────────────────────

    def test_get_task_count_default_zero(self):
        self.assertEqual(self.store.get_task_count(), 0)

    def test_set_and_get_task_count(self):
        self.store.set_task_count(7)
        self.assertEqual(self.store.get_task_count(), 7)

    def test_task_count_survives_new_store_instance(self):
        self.store.set_task_count(12)
        store2 = ConversationStore(self.tmpdir)
        self.assertEqual(store2.get_task_count(), 12)

    def test_task_count_coexists_with_model_pref(self):
        self.store.set_preferred_model("claude-opus-4-6")
        self.store.set_task_count(3)
        self.assertEqual(self.store.get_preferred_model(), "claude-opus-4-6")
        self.assertEqual(self.store.get_task_count(), 3)

    # ── rotation ─────────────────────────────────────────────────────

    def test_rotation_at_max_conversations(self):
        for i in range(MAX_CONVERSATIONS + 5):
            self.store.new_conversation(f"m{i}")
        convs = self.store.list_conversations()
        self.assertLessEqual(len(convs), MAX_CONVERSATIONS)

    def test_rotation_removes_oldest_files(self):
        ids = []
        for i in range(MAX_CONVERSATIONS + 3):
            conv = self.store.new_conversation(f"m{i}")
            ids.append(conv["id"])
        # Oldest conversations should have their files removed
        oldest_path = Path(self.tmpdir) / "conversations" / f"{ids[0]}.json"
        self.assertFalse(oldest_path.exists())

    # ── index recovery ───────────────────────────────────────────────

    def test_malformed_index_recovers(self):
        index_path = Path(self.tmpdir) / "conversations" / "_index.json"
        index_path.write_text("not json{{{")
        # Should recover gracefully
        convs = self.store.list_conversations()
        self.assertEqual(convs, [])
        # And new conversations still work
        conv = self.store.new_conversation("m")
        self.assertIsNotNone(conv["id"])


if __name__ == "__main__":
    unittest.main()
