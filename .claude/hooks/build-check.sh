#!/bin/bash
# PostToolUse hook (Edit/Write): remind to build/test after Go file changes.
# Hook input is JSON on stdin.

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

if ! echo "$FILE_PATH" | grep -qE '\.go$'; then
  exit 0
fi

jq -n '{
  hookSpecificOutput: {
    additionalContext: "Go file modified. Run `make build` or `make test` to verify."
  }
}'
exit 0
