# Troubleshooting

Common problems and how to fix them.

## Daemon won't start

**Symptom:** `mnemonic start` hangs or exits immediately.

1. Check if it's already running:
   ```bash
   mnemonic status
   ```
2. Try foreground mode to see errors directly:
   ```bash
   mnemonic serve
   ```
3. Check the log file:
   ```bash
   cat ~/.mnemonic/mnemonic.log
   ```
4. Run diagnostics:
   ```bash
   mnemonic diagnose
   ```

**Common causes:**
- Port 9999 already in use — another mnemonic instance or another service. Check with `lsof -i :9999`.
- Bad `config.yaml` — run `mnemonic diagnose` to validate.
- Missing database directory — the daemon creates `~/.mnemonic/` automatically, but check permissions.

## Memories not encoding

**Symptom:** Raw memories appear (`mnemonic status` shows raw count growing) but encoded memory count stays flat.

1. Check LLM connectivity:
   ```bash
   mnemonic diagnose
   ```
   Look for "LLM: healthy" in output.

2. Verify LM Studio is running with the correct models loaded:
   - Chat model must match `llm.chat_model` in config.yaml
   - Embedding model must match `llm.embedding_model` in config.yaml
   - See [LM Studio Setup](setup-lmstudio.md)

3. Check for encoding errors in the log:
   ```bash
   grep -i "encoding" ~/.mnemonic/mnemonic.log | tail -20
   ```

4. If the LLM was down for a while, unprocessed raw memories will be picked up on the next encoding cycle (runs every few minutes when the daemon is healthy).

## Search returns nothing or irrelevant results

**Symptom:** `mnemonic recall "query"` returns empty or off-topic results.

1. Verify memories exist:
   ```bash
   mnemonic status
   ```
   Check that "active memories" count is > 0.

2. Try broader queries — retrieval uses both keyword (FTS) and semantic (embedding) search.

3. If you recently changed the LLM embedding model, existing embeddings are incompatible with the new model. The daemon warns about this at startup ("embedding model drift detected"). You may need to re-encode memories or revert to the previous model.

4. Provide feedback to help the system learn:
   ```bash
   # Through the MCP tools or web dashboard, rate recall quality
   # "helpful", "partial", or "irrelevant"
   ```

## High CPU usage

**Symptom:** The mnemonic process is using significant CPU continuously.

1. Check if a consolidation or dream cycle is running — these are CPU-intensive but temporary (usually < 5 minutes).

2. Check LLM concurrency — if `llm.max_concurrent` is too high for your machine, reduce it:
   ```yaml
   llm:
     max_concurrent: 1  # lower for less powerful machines
   ```

3. Filesystem watcher scanning too many files — narrow your watch directories:
   ```yaml
   perception:
     filesystem:
       watch_dirs:
         - "~/Projects"      # specific directories, not ~
         - "~/Documents"
   ```

4. If CPU stays high after 10+ minutes, check the log for error loops:
   ```bash
   tail -100 ~/.mnemonic/mnemonic.log | grep -i error
   ```

## Disk space growing

**Symptom:** `~/.mnemonic/memory.db` is getting large.

1. Check database size:
   ```bash
   ls -lh ~/.mnemonic/memory.db
   ```

2. The daemon monitors disk space and warns at startup when free space drops below 500MB.

3. Old backups accumulate in `~/.mnemonic/backups/`. The `backup` command keeps only the last 5, but manual cleanup may help:
   ```bash
   ls -la ~/.mnemonic/backups/
   ```

4. Run consolidation to merge and archive stale memories:
   ```bash
   mnemonic consolidate
   ```

5. SQLite WAL files (`memory.db-wal`) can grow temporarily during writes. They are checkpointed automatically.

## LM Studio connection errors

**Symptom:** `mnemonic diagnose` reports LLM as unhealthy.

1. Verify LM Studio is running and the server is started (not just the app).
2. Check the endpoint matches your config:
   ```yaml
   llm:
     endpoint: "http://localhost:1234/v1"  # default LM Studio port
   ```
3. Ensure both models are loaded in LM Studio:
   - A chat model (e.g., `qwen/qwen3.5-9b`)
   - An embedding model (e.g., `text-embedding-embeddinggemma-300m-qat`)
4. See [LM Studio Setup](setup-lmstudio.md) for detailed configuration.

## Database corruption

**Symptom:** `mnemonic diagnose` reports integrity check failures, or you see "database corruption detected" at startup.

1. The daemon runs `PRAGMA integrity_check` at startup and will warn if corruption is found.

2. Restore from the most recent backup:
   ```bash
   ls ~/.mnemonic/backups/
   mnemonic restore ~/.mnemonic/backups/pre_migrate_2026-03-10_143022.db
   ```

3. If no backup exists, you can try SQLite's built-in recovery:
   ```bash
   sqlite3 ~/.mnemonic/memory.db ".recover" | sqlite3 ~/.mnemonic/memory_recovered.db
   ```

4. As a last resort, `mnemonic purge` deletes everything and starts fresh.

See [Backup & Restore](backup-restore.md) for the full disaster recovery procedure.

## Configuration errors

**Symptom:** Exit code 2 at startup.

1. Run diagnostics:
   ```bash
   mnemonic diagnose
   ```

2. Common config issues:
   - Watch directory set to home directory (`~`) — use specific subdirectories instead
   - Model names don't match what's loaded in LM Studio
   - Invalid YAML syntax — check for tab characters (YAML requires spaces)

3. The default config location is `config.yaml` in the working directory. Override with:
   ```bash
   mnemonic --config /path/to/config.yaml serve
   ```

## Agent Chat won't connect

**Symptom:** The Agent Chat panel shows "Connection error", "Failed to connect", or tasks fail immediately with "Internal server error" or "Setup required".

### Claude CLI not authenticated

The agent chat runs via the Claude Agent SDK, which spawns the `claude` CLI as a subprocess and uses your Claude subscription via OAuth. If you haven't logged in on this machine, tasks will fail.

**Fix:**
```bash
claude   # opens OAuth flow in your browser; credentials are saved to ~/.claude/
```

After authenticating, reload the dashboard. No daemon restart needed — the Python WebSocket server picks up the stored credentials automatically.

### Claude CLI not installed

If you see "Claude CLI not found":
```bash
npm install -g @anthropic-ai/claude-code
```

### Agent Chat panel not visible

The agent chat requires `agent_sdk.enabled: true` and a valid `evolution_dir` in `config.yaml`:
```yaml
agent_sdk:
  enabled: true
  evolution_dir: "./sdk/agent/evolution"
  web_port: 9998
```

Restart the daemon after changing config.

### Python WebSocket server not starting

The agent WebSocket server (port 9998) is a Python subprocess. The daemon looks for a Python binary in this order:

1. `agent_sdk.python_bin` in config (if set)
2. `sdk/.venv/bin/python3` — the project venv, preferred when it exists
3. `uv` in PATH
4. `python3` in PATH

If the server isn't starting, check whether it started in the daemon log:
```bash
journalctl --user -u mnemonic -n 50 | grep -i agent
```

Or run in foreground to see Python output directly:
```bash
mnemonic serve
```

If you see an import error (`ModuleNotFoundError`), the venv is missing or not installed. Set up the venv from the `sdk/` directory:
```bash
cd sdk && python3 -m venv .venv && source .venv/bin/activate && pip install -e .
```

## Permission errors

**Symptom:** Exit code 5 at startup.

1. Check that `~/.mnemonic/` is writable:
   ```bash
   ls -la ~/.mnemonic/
   ```

2. On macOS, the daemon may need Full Disk Access for filesystem watching. Go to System Settings > Privacy & Security > Full Disk Access.

3. For the system service (`mnemonic install`), ensure the LaunchAgent plist or systemd unit has correct paths.
