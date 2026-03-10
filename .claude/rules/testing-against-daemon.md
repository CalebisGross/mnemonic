# Testing Against the Running Daemon

## The Daemon Is Always Running

Mnemonic runs as a systemd user service (`mnemonic.service`), started at boot. Assume it is running unless told otherwise. After testing, always leave the daemon in a running state.

## After Any Code Change

The running daemon serves the old binary until you rebuild and restart:

1. `make build` — rebuild the binary
2. `systemctl --user restart mnemonic` — restart the daemon to pick up changes
3. Verify the fix is live — hit the API, check the dashboard (`http://127.0.0.1:9999/`), or exercise whatever was changed

This applies to all changes: Go code, embedded assets (dashboard HTML via `//go:embed`), API routes, MCP tools, agent logic — everything is compiled into a single binary.
