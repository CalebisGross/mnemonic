from __future__ import annotations

from typing import Any

_nudged_files: set[str] = set()
_MAX_WRITE_NUDGES = 5


def reset_nudge_state() -> None:
    """Clear nudge tracking state. Call between tests for isolation."""
    _nudged_files.clear()


async def post_tool_use_hook(
    input_data: dict[str, Any],
    tool_use_id: str | None,
    context: Any,
) -> dict[str, Any]:
    """PostToolUse hook that nudges Claude to capture memories at key moments.

    Returns systemMessage strings that get injected into the conversation,
    guiding Claude toward recording the right things. Never calls tools
    directly — preserves Claude's agency and avoids infinite loops.

    Rate-limited: Write/Edit nudges fire at most _MAX_WRITE_NUDGES times
    per session and deduplicate by file path.
    """
    tool_name = input_data.get("tool_name", "")
    tool_input = input_data.get("tool_input", {})
    tool_response = input_data.get("tool_response", {})

    # After file writes/edits — nudge decision capture (rate-limited)
    if tool_name in ("Write", "Edit"):
        file_path = tool_input.get("file_path", "unknown")
        if file_path in _nudged_files or len(_nudged_files) >= _MAX_WRITE_NUDGES:
            return {}
        _nudged_files.add(file_path)
        return {
            "systemMessage": (
                f"You just modified {file_path}. "
                "If this reflects an architectural decision or non-obvious choice, "
                "call mcp__mnemonic__remember with type='decision' to preserve the rationale."
            )
        }

    # After Bash failures — nudge error pattern capture
    if tool_name == "Bash":
        exit_code = tool_response.get("exitCode", 0)
        if exit_code != 0:
            output = tool_response.get("output", "")
            tail = output[-300:] if len(output) > 300 else output
            return {
                "systemMessage": (
                    f"Bash command failed (exit code {exit_code}). Tail: {tail}\n"
                    "If you identify and fix the root cause, call mcp__mnemonic__remember "
                    "with type='error' to record the fix pattern for future reference."
                )
            }

    # After recall — nudge feedback
    if tool_name == "mcp__mnemonic__recall":
        return {
            "systemMessage": (
                "You just performed a recall. After reviewing the results, "
                "call mcp__mnemonic__feedback with quality='helpful', 'partial', or 'irrelevant' "
                "to help the memory system learn."
            )
        }

    return {}
