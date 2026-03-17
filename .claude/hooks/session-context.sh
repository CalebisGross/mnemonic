#!/bin/bash
# SessionStart hook: Inject project context at session start.
# Hook input is JSON on stdin.

cat > /dev/null  # consume stdin

jq -n '{
  hookSpecificOutput: {
    hookEventName: "SessionStart",
    additionalContext: "MNEMONIC SESSION START: You MUST call recall_project and recall (with relevant keywords) before doing any work. Check GitHub issues (gh issue list) for current priorities. Platform: macOS (full), Linux (partial — daemon needs systemd). Use `make serve` for foreground mode on Linux."
  }
}'
exit 0
