#!/bin/bash
# PostToolUse hook: after editing Go files, remind about lint rules.
# This prevents the #1 source of CI failures: unchecked error returns.

INPUT=$(cat)
TOOL=$(echo "$INPUT" | jq -r '.tool_name // empty')
FILE_PATH=$(echo "$INPUT" | jq -r '.tool_input.file_path // empty')

# Only trigger for Edit/Write on Go files
if [ "$TOOL" != "Edit" ] && [ "$TOOL" != "Write" ]; then
  exit 0
fi

if ! echo "$FILE_PATH" | grep -qE '\.go$'; then
  exit 0
fi

jq -n '{
  hookSpecificOutput: {
    additionalContext: "LINT REMINDER: All error return values MUST be handled. Use `if err := ...; err != nil { ... }` for errors that matter, or `_ = expr` to explicitly discard. Never leave error returns unchecked — errcheck enforced in CI. Also: no unused types/vars/funcs, use strings.ContainsAny not IndexAny, use struct conversions where types match."
  }
}'
exit 0
