"""Tests for ConversationStore."""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from agent.conversation_store import (
    ASSISTANT_TRUNCATE,
    MAX_CONVERSATIONS,
    MAX_SUMMARY_CHARS,
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

    # ── build_continuation_summary ───────────────────────────────────

    def test_continuation_summary_returns_none_for_missing(self):
        self.assertIsNone(self.store.build_continuation_summary("no-such"))

    def test_continuation_summary_includes_messages(self):
        conv = self.store.new_conversation("m")
        self.store.append_message(conv["id"], {"role": "user", "content": "fix bug"})
        self.store.append_message(conv["id"], {"role": "assistant", "content": "done"})
        summary = self.store.build_continuation_summary(conv["id"])
        self.assertIn("User: fix bug", summary)
        self.assertIn("Assistant: done", summary)
        self.assertIn("continuing this conversation", summary)

    def test_continuation_summary_truncates_long_assistant(self):
        conv = self.store.new_conversation("m")
        long_response = "x" * (ASSISTANT_TRUNCATE + 100)
        self.store.append_message(conv["id"], {"role": "assistant", "content": long_response})
        summary = self.store.build_continuation_summary(conv["id"])
        self.assertIn("x" * ASSISTANT_TRUNCATE + "...", summary)
        self.assertNotIn("x" * (ASSISTANT_TRUNCATE + 1), summary)

    def test_continuation_summary_caps_total_length(self):
        conv = self.store.new_conversation("m")
        # Add many messages to exceed MAX_SUMMARY_CHARS
        for i in range(100):
            self.store.append_message(conv["id"], {
                "role": "user",
                "content": f"message {i} " + "x" * 100,
            })
        summary = self.store.build_continuation_summary(conv["id"])
        self.assertLessEqual(len(summary), MAX_SUMMARY_CHARS + 500)  # allow for header/footer
        self.assertIn("earlier messages trimmed", summary)

    def test_continuation_summary_includes_tool_use(self):
        conv = self.store.new_conversation("m")
        self.store.append_message(conv["id"], {"role": "tool_use", "name": "Read"})
        summary = self.store.build_continuation_summary(conv["id"])
        self.assertIn("[Tool: Read]", summary)

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
