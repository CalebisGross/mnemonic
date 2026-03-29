#!/bin/bash
# PostToolUse hook (Edit/Write): remind about lint rules after Go file changes.
# This prevents the #1 source of CI failures: unchecked error returns.

INPUT=$(cat)
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

if ! echo "$FILE_PATH" | grep -qE '\.go$'; then
  exit 0
fi

jq -n '{
  hookSpecificOutput: {
    additionalContext: "LINT REMINDER: All error return values MUST be handled. Use `if err := ...; err != nil { ... }` for errors that matter, or `_ = expr` to explicitly discard. Never leave error returns unchecked — errcheck enforced in CI. Also: no unused types/vars/funcs, use strings.ContainsAny not IndexAny, use struct conversions where types match."
  }
}'
exit 0
