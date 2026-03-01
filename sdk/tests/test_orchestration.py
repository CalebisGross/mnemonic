"""Tests for the orchestration layer: build_options, make_subagents, Config, prompt budget."""

import asyncio
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import AsyncMock, MagicMock, patch

from agent.config import Config
from agent.hooks import reset_nudge_state
from agent.prompts import (
    _PRINCIPLES_MAX,
    _PRINCIPLES_MIN_CONFIDENCE,
    _PROMPT_BUDGET,
    assemble_system_prompt,
)
from agent.tools import ALL_MNEMONIC_TOOLS, BUILTIN_TOOLS, MNEMONIC_RECALL_TOOLS


class TestToolLists(unittest.TestCase):
    def test_all_mnemonic_tools_has_12_entries(self):
        self.assertEqual(len(ALL_MNEMONIC_TOOLS), 12)

    def test_builtin_tools_has_7_entries(self):
        self.assertEqual(len(BUILTIN_TOOLS), 7)

    def test_recall_tools_is_subset_of_all(self):
        for tool in MNEMONIC_RECALL_TOOLS:
            self.assertIn(tool, ALL_MNEMONIC_TOOLS)

    def test_no_duplicates_in_all_mnemonic_tools(self):
        self.assertEqual(len(ALL_MNEMONIC_TOOLS), len(set(ALL_MNEMONIC_TOOLS)))


class TestConfig(unittest.TestCase):
    def test_default_evolution_dir_uses_sdk_dir(self):
        cfg = Config()
        self.assertTrue(cfg.evolution_dir.endswith("evolution"))

    def test_evolution_dir_override_from_field(self):
        cfg = Config(evolution_dir_override="/custom/path")
        self.assertEqual(cfg.evolution_dir, "/custom/path")

    def test_evolution_dir_override_from_env(self):
        with patch.dict(os.environ, {"MNEMONIC_EVOLUTION_DIR": "/env/path"}):
            cfg = Config()
            self.assertEqual(cfg.evolution_dir, "/env/path")

    def test_project_root_from_config_path(self):
        cfg = Config(mnemonic_config="/some/project/config.yaml")
        self.assertEqual(cfg.project_root, "/some/project")

    def test_default_model(self):
        with patch.dict(os.environ, {}, clear=True):
            # Remove env var if set
            os.environ.pop("MNEMONIC_AGENT_MODEL", None)
            os.environ.pop("MNEMONIC_EVOLUTION_DIR", None)
            cfg = Config()
            self.assertEqual(cfg.model, "claude-sonnet-4-6")


class TestBuildOptions(unittest.TestCase):
    @patch("agent.options.assemble_system_prompt", return_value="test prompt")
    @patch("agent.options.make_subagents", return_value={})
    def test_includes_all_tools(self, _mock_sub, _mock_prompt):
        from agent.options import build_options

        cfg = Config()
        opts = build_options(cfg)
        expected = BUILTIN_TOOLS + ALL_MNEMONIC_TOOLS
        self.assertEqual(opts.allowed_tools, expected)

    @patch("agent.options.assemble_system_prompt", return_value="test prompt")
    @patch("agent.options.make_subagents", return_value={})
    def test_permission_mode_passed(self, _mock_sub, _mock_prompt):
        from agent.options import build_options

        cfg = Config(permission_mode="bypassPermissions")
        opts = build_options(cfg)
        self.assertEqual(opts.permission_mode, "bypassPermissions")

    @patch("agent.options.assemble_system_prompt", return_value="test prompt")
    @patch("agent.options.make_subagents", return_value={})
    def test_max_turns_omitted_when_none(self, _mock_sub, _mock_prompt):
        from agent.options import build_options

        cfg = Config(max_turns=None)
        opts = build_options(cfg)
        self.assertFalse(hasattr(opts, "max_turns") and opts.max_turns is not None)

    @patch("agent.options.assemble_system_prompt", return_value="test prompt")
    @patch("agent.options.make_subagents", return_value={})
    def test_max_turns_set_when_provided(self, _mock_sub, _mock_prompt):
        from agent.options import build_options

        cfg = Config(max_turns=10)
        opts = build_options(cfg)
        self.assertEqual(opts.max_turns, 10)


class TestMakeSubagents(unittest.TestCase):
    def test_returns_three_agents(self):
        from agent.subagents import make_subagents

        agents = make_subagents()
        self.assertEqual(len(agents), 3)
        self.assertIn("code-reviewer", agents)
        self.assertIn("test-runner", agents)
        self.assertIn("memory-archivist", agents)

    def test_model_propagated(self):
        from agent.subagents import make_subagents

        agents = make_subagents(subagent_model="opus")
        for name, defn in agents.items():
            self.assertEqual(defn.model, "opus", f"{name} should use opus")

    def test_code_reviewer_has_read_and_recall_tools(self):
        from agent.subagents import make_subagents

        agents = make_subagents()
        tools = agents["code-reviewer"].tools
        self.assertIn("Read", tools)
        self.assertIn("mcp__mnemonic__recall", tools)

    def test_memory_archivist_has_all_mnemonic_tools(self):
        from agent.subagents import make_subagents

        agents = make_subagents()
        for tool in ALL_MNEMONIC_TOOLS:
            self.assertIn(tool, agents["memory-archivist"].tools)


class TestPromptBudget(unittest.TestCase):
    def test_principles_filtered_by_confidence(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "principles.yaml").write_text(
                "principles:\n"
                '  - id: p1\n    text: "High conf"\n    source: "test"\n    confidence: 0.8\n'
                '  - id: p2\n    text: "Low conf"\n    source: "test"\n    confidence: 0.3\n'
                '  - id: p3\n    text: "Exact threshold"\n    source: "test"\n    confidence: 0.6\n'
            )
            (Path(tmpdir) / "strategies.yaml").write_text("strategies: {}\n")
            (Path(tmpdir) / "prompt_patches.yaml").write_text("patches: []\n")

            prompt = assemble_system_prompt(tmpdir)
            self.assertIn("High conf", prompt)
            self.assertNotIn("Low conf", prompt)
            self.assertIn("Exact threshold", prompt)

    def test_principles_sorted_by_confidence_desc(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "principles.yaml").write_text(
                "principles:\n"
                '  - id: p1\n    text: "Medium"\n    source: "test"\n    confidence: 0.7\n'
                '  - id: p2\n    text: "Highest"\n    source: "test"\n    confidence: 0.9\n'
                '  - id: p3\n    text: "High"\n    source: "test"\n    confidence: 0.8\n'
            )
            (Path(tmpdir) / "strategies.yaml").write_text("strategies: {}\n")
            (Path(tmpdir) / "prompt_patches.yaml").write_text("patches: []\n")

            prompt = assemble_system_prompt(tmpdir)
            # Highest should appear before Medium
            idx_highest = prompt.index("Highest")
            idx_medium = prompt.index("Medium")
            self.assertLess(idx_highest, idx_medium)

    def test_principles_capped_at_max(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            lines = ["principles:"]
            for i in range(_PRINCIPLES_MAX + 5):
                lines.append(
                    f'  - id: p{i}\n    text: "Principle {i}"\n'
                    f'    source: "test"\n    confidence: 0.8\n'
                )
            (Path(tmpdir) / "principles.yaml").write_text("\n".join(lines))
            (Path(tmpdir) / "strategies.yaml").write_text("strategies: {}\n")
            (Path(tmpdir) / "prompt_patches.yaml").write_text("patches: []\n")

            prompt = assemble_system_prompt(tmpdir)
            # Should have exactly _PRINCIPLES_MAX entries
            count = prompt.count("[0.8]")
            self.assertEqual(count, _PRINCIPLES_MAX)

    def test_strategies_omit_tips(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "principles.yaml").write_text("principles: []\n")
            (Path(tmpdir) / "strategies.yaml").write_text(
                "strategies:\n  bug_fix:\n    steps:\n"
                '      - "Reproduce first"\n'
                "    tips:\n"
                '      - "Check logs early"\n'
            )
            (Path(tmpdir) / "prompt_patches.yaml").write_text("patches: []\n")

            prompt = assemble_system_prompt(tmpdir)
            self.assertIn("Reproduce first", prompt)
            self.assertNotIn("Check logs early", prompt)
            self.assertIn("bug_fix", prompt)

    def test_patches_capped(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            (Path(tmpdir) / "principles.yaml").write_text("principles: []\n")
            (Path(tmpdir) / "strategies.yaml").write_text("strategies: {}\n")
            lines = ["patches:"]
            for i in range(25):
                lines.append(f'  - id: pp{i}\n    instruction: "Patch {i}"\n    reason: "test"\n')
            (Path(tmpdir) / "prompt_patches.yaml").write_text("\n".join(lines))

            prompt = assemble_system_prompt(tmpdir)
            # Count occurrences — only first 20 should appear
            count = sum(1 for i in range(25) if f"Patch {i}" in prompt)
            self.assertEqual(count, 20)


class TestHookRateLimit(unittest.TestCase):
    def setUp(self):
        reset_nudge_state()

    def tearDown(self):
        reset_nudge_state()

    def _run(self, coro):
        return asyncio.run(coro)

    def test_same_file_nudges_once(self):
        from agent.hooks import post_tool_use_hook

        r1 = self._run(post_tool_use_hook(
            {"tool_name": "Write", "tool_input": {"file_path": "/tmp/same.py"}, "tool_response": {}},
            "tu_1", None,
        ))
        r2 = self._run(post_tool_use_hook(
            {"tool_name": "Write", "tool_input": {"file_path": "/tmp/same.py"}, "tool_response": {}},
            "tu_2", None,
        ))
        self.assertIn("systemMessage", r1)
        self.assertEqual(r2, {})

    def test_different_files_each_nudge(self):
        from agent.hooks import post_tool_use_hook

        for i in range(3):
            result = self._run(post_tool_use_hook(
                {"tool_name": "Edit", "tool_input": {"file_path": f"/tmp/file{i}.py"}, "tool_response": {}},
                f"tu_{i}", None,
            ))
            self.assertIn("systemMessage", result)

    def test_cap_at_max_nudges(self):
        from agent.hooks import _MAX_WRITE_NUDGES, post_tool_use_hook

        for i in range(_MAX_WRITE_NUDGES + 3):
            result = self._run(post_tool_use_hook(
                {"tool_name": "Write", "tool_input": {"file_path": f"/tmp/cap{i}.py"}, "tool_response": {}},
                f"tu_{i}", None,
            ))
            if i < _MAX_WRITE_NUDGES:
                self.assertIn("systemMessage", result, f"Nudge {i} should fire")
            else:
                self.assertEqual(result, {}, f"Nudge {i} should be suppressed")


class TestStreamEvents(unittest.TestCase):
    def test_text_and_done_events(self):
        from claude_agent_sdk import AssistantMessage, ResultMessage, TextBlock

        from agent.session import stream_events

        async def fake_messages():
            yield AssistantMessage(
                content=[TextBlock(text="hello")],
                model="test-model",
            )
            yield ResultMessage(
                subtype="success",
                duration_ms=100,
                duration_api_ms=80,
                is_error=False,
                num_turns=1,
                session_id="test-session",
                total_cost_usd=0.01,
            )

        mock_client = MagicMock()
        mock_client.receive_messages = fake_messages

        events = []

        async def collect():
            async for ev in stream_events(mock_client):
                events.append(ev)

        asyncio.run(collect())
        types = [e[0] for e in events]
        self.assertIn("text", types)
        self.assertIn("done", types)
        text_events = [e for e in events if e[0] == "text"]
        self.assertEqual(text_events[0][1], "hello")
        done_events = [e for e in events if e[0] == "done"]
        cost, turns = done_events[0][1]
        self.assertAlmostEqual(cost, 0.01)
        self.assertEqual(turns, 1)

    def test_thinking_event(self):
        from claude_agent_sdk import AssistantMessage, ResultMessage, ThinkingBlock

        from agent.session import stream_events

        async def fake_messages():
            yield AssistantMessage(
                content=[ThinkingBlock(thinking="deep thought", signature="sig")],
                model="test-model",
            )
            yield ResultMessage(
                subtype="success", duration_ms=0, duration_api_ms=0,
                is_error=False, num_turns=0, session_id="s",
            )

        mock_client = MagicMock()
        mock_client.receive_messages = fake_messages

        events = []

        async def collect():
            async for ev in stream_events(mock_client):
                events.append(ev)

        asyncio.run(collect())
        thinking_events = [e for e in events if e[0] == "thinking"]
        self.assertEqual(len(thinking_events), 1)
        # Thinking is truncated to 300 chars
        self.assertEqual(thinking_events[0][1], "deep thought")

    def test_tool_use_event(self):
        from claude_agent_sdk import AssistantMessage, ResultMessage, ToolUseBlock

        from agent.session import stream_events

        async def fake_messages():
            yield AssistantMessage(
                content=[ToolUseBlock(id="tu_1", name="Read", input={"file_path": "/tmp/x"})],
                model="test-model",
            )
            yield ResultMessage(
                subtype="success", duration_ms=0, duration_api_ms=0,
                is_error=False, num_turns=0, session_id="s",
            )

        mock_client = MagicMock()
        mock_client.receive_messages = fake_messages

        events = []

        async def collect():
            async for ev in stream_events(mock_client):
                events.append(ev)

        asyncio.run(collect())
        tool_events = [e for e in events if e[0] == "tool_use"]
        self.assertEqual(len(tool_events), 1)
        name, inp = tool_events[0][1]
        self.assertEqual(name, "Read")


if __name__ == "__main__":
    unittest.main()
