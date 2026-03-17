# Contributing to Mnemonic

Thanks for your interest in contributing! Mnemonic is a local-first semantic memory system built in Go, and we welcome contributions of all kinds.

## Getting Started

### Prerequisites

- Go 1.23+
- A C compiler (CGO is required for SQLite)
- `golangci-lint` for linting

### Build from Source

```bash
git clone https://github.com/appsprout-dev/mnemonic.git
cd mnemonic
make setup-hooks   # Configure pre-commit hooks
make build         # Compile binary to bin/mnemonic
make test          # Run full test suite
```

The Makefile handles all build configuration automatically. No special flags needed.

## Development Workflow

1. Fork the repo and clone your fork
2. Create a feature branch from `main`: `git checkout -b feat/your-feature`
3. Make your changes
4. Run checks:
   ```bash
   make check       # go fmt + go vet
   make lint        # golangci-lint
   make test        # full test suite
   ```
5. Commit using [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat: add memory source tracking` (new feature)
   - `fix: prevent nil pointer in retrieval` (bug fix)
   - `docs: update README` (documentation)
   - `refactor: simplify consolidation loop` (no behavior change)
   - `test: add encoding agent coverage` (tests only)
   - `chore: update dependencies` (maintenance)
6. Push your branch and open a PR against `main`

## Code Conventions

- **Error handling**: Wrap errors with context — `fmt.Errorf("encoding memory %s: %w", id, err)`
- **errcheck**: Every error return must be handled. Use `_ = expr` for intentionally ignored errors (best-effort cleanup, fire-and-forget events)
- **Platform code**: Use build tags (`//go:build darwin`, `//go:build linux`), not runtime checks. Every platform-specific file needs a counterpart for other platforms
- **Architecture**: Agents communicate via the event bus (`internal/events/`), never direct calls. All data access goes through the `store.Store` interface
- **Tests**: Place test files next to the code they test. Use table-driven tests where applicable

## Project Structure

See [CLAUDE.md](CLAUDE.md) for the full project layout. Key directories:

- `cmd/mnemonic/` — CLI entry point
- `internal/agent/` — Cognitive agents (perception, encoding, retrieval, etc.)
- `internal/store/` — Store interface + SQLite implementation
- `internal/api/` — REST API server
- `internal/mcp/` — MCP server (13 tools for Claude Code)

## What to Work On

Check [GitHub Issues](https://github.com/appsprout-dev/mnemonic/issues) for open bugs and feature requests. Issues labeled `good first issue` are a great starting point.

## Reporting Bugs

Open an issue with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Output of `mnemonic diagnose` (if applicable)
- Your platform (macOS/Linux) and Go version

## License

By contributing, you agree that your contributions will be licensed under the [AGPL-3.0 License](LICENSE).
