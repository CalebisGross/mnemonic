# Backup & Restore

Mnemonic stores all data in a single SQLite database at `~/.mnemonic/memory.db`. This guide covers backup strategies and disaster recovery.

## Quick Backup

```bash
mnemonic backup
```

This creates a timestamped JSON export in `~/.mnemonic/backups/` and automatically keeps only the last 5 backups.

## Backup Types

### JSON Export

Exports memories, associations, and raw memories as structured JSON:

```bash
mnemonic export --format json --output ~/my-backup.json
```

**Pros:** Human-readable, importable, format-stable across versions.
**Cons:** Doesn't include SQLite-specific data (patterns, abstractions, episodes, system metadata).

### SQLite File Copy

Copies the raw database file:

```bash
mnemonic export --format sqlite --output ~/my-backup.db
```

**Pros:** Complete copy of all data including patterns, abstractions, and indexes.
**Cons:** May not be compatible across major schema changes.

### Automatic Pre-Migration Backups

The daemon automatically creates a SQLite backup before applying schema migrations at startup. These are saved as `pre_migrate_<timestamp>.db` in `~/.mnemonic/backups/`.

## Restore from Backup

### Restore a SQLite Backup

```bash
mnemonic restore ~/.mnemonic/backups/pre_migrate_2026-03-10_143022.db
```

This will:
1. Verify the backup file exists and passes `PRAGMA integrity_check`
2. Check that the daemon is not running (stop it first with `mnemonic stop`)
3. Move the current database aside as `memory.db.old`
4. Copy the backup into place as the new `memory.db`

After restoring, start the daemon:
```bash
mnemonic start
```

### Import a JSON Backup

```bash
mnemonic import ~/my-backup.json --mode merge
```

Modes:
- `merge` — Add imported memories alongside existing ones (default)
- `replace` — Clear existing data and load from backup

## Disaster Recovery

### Database Deleted

If `~/.mnemonic/memory.db` is accidentally deleted:

1. Check for automatic backups:
   ```bash
   ls -lt ~/.mnemonic/backups/
   ```

2. Restore the most recent one:
   ```bash
   mnemonic restore ~/.mnemonic/backups/<most-recent>.db
   ```

3. If no backups exist, start fresh:
   ```bash
   mnemonic serve   # creates a new empty database
   ```
   Then re-ingest important projects:
   ```bash
   mnemonic ingest ~/Projects/my-project --project my-project
   ```

### Database Corrupted

If `mnemonic diagnose` reports corruption:

1. Try restoring from a pre-migration backup (created automatically):
   ```bash
   mnemonic restore ~/.mnemonic/backups/pre_migrate_<latest>.db
   ```

2. Try SQLite's built-in recovery:
   ```bash
   sqlite3 ~/.mnemonic/memory.db ".recover" | sqlite3 /tmp/recovered.db
   mnemonic restore /tmp/recovered.db
   ```

3. Last resort — purge and start over:
   ```bash
   mnemonic purge
   ```

### Embedding Model Changed

If you switched LLM embedding models, existing vector embeddings are incompatible:

1. The daemon detects this and warns at startup.
2. Existing memories still work for keyword search (FTS) but semantic search quality degrades.
3. Options:
   - Switch back to the original model
   - Accept degraded semantic search for old memories (new ones will use the new model)
   - `mnemonic purge` and re-ingest if semantic quality is critical

## Backup Schedule Recommendations

| Use Case | Strategy |
|----------|----------|
| Personal use | Run `mnemonic backup` weekly |
| Heavy use | Run `mnemonic backup` daily via cron |
| Before upgrades | The daemon does this automatically (pre-migration backup) |
| Before experiments | Manual `mnemonic export --format sqlite --output ~/before-experiment.db` |

### Automated Daily Backup (cron)

```bash
# Add to crontab: crontab -e
0 3 * * * /path/to/mnemonic --config /path/to/config.yaml backup
```

## Backup Location

All backups are stored in `~/.mnemonic/backups/`. The `backup` command automatically manages retention (keeps last 5). Pre-migration backups are also stored here but are not subject to the retention limit — clean them up manually if needed.
