#!/bin/bash
# After editing Go files, remind about build requirements.
# Hook input is JSON on stdin.

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
    additionalContext: "Go file modified. Run `make build` or `make test` to verify."
  }
}'
exit 0
