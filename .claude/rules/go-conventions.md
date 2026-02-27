# Go Conventions

## Build Requirements

- ALL builds: `CGO_ENABLED=1` (for SQLite via mattn/go-sqlite3)
- ALL builds: `-tags sqlite_fts5` (for full-text search)
- Version injected via ldflags: `-X main.Version=$(VERSION)`
- Use `make build` / `make test` to get these right automatically

## Code Style

- Run `go fmt ./...` before committing (enforced by pre-commit hook)
- Run `go vet ./...` before committing (enforced by pre-commit hook)
- `golangci-lint run` must pass (enforced by CI and pre-commit hook)
- Error wrapping: `fmt.Errorf("doing thing %s: %w", id, err)`
- No naked returns in functions with named return values

## Lint Rules (errcheck is the #1 source of CI failures)

- EVERY error return value must be handled — no exceptions
- If the error matters: `if err := expr; err != nil { log.Warn(...) }` or `return err`
- If the error is ignorable (best-effort cleanup, fire-and-forget): `_ = expr`
- Common patterns:
  - `_ = bus.Publish(...)` — event notifications are fire-and-forget
  - `_ = tx.Rollback()` — no-op after successful commit
  - `_ = json.Unmarshal(...)` — when parsing app-written JSON in DB rows
  - `if err := store.Write(...); err != nil { log.Warn(...) }` — store writes should be logged
- Never leave unused types, consts, fields, or functions — remove them
- Use `strings.ContainsAny()` not `strings.IndexAny() == -1`
- Use struct conversion `TargetType(source)` when field names/types match

## Architecture

- Agents communicate via event bus (`internal/events/`), NEVER direct calls
- All data access through `store.Store` interface -- never import sqlite package directly from agents
- Config additions go in `internal/config/config.go` struct + `config.yaml`
- Platform-specific code uses build tags: `//go:build darwin` / `//go:build !darwin`

## Testing

- Tests require the same build tags: `CGO_ENABLED=1 go test -tags sqlite_fts5 ./...`
- Test files go next to the code they test: `foo_test.go` alongside `foo.go`
- Use table-driven tests where applicable

## Module Path

- Go module: `github.com/appsprout/mnemonic` (this is the company module path, keep it)
- Import paths follow this: `github.com/appsprout/mnemonic/internal/...`
