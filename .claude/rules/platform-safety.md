# Platform Safety — Never Break the User

## The Rule

Never ship code that breaks an existing platform. If it works on macOS today, it must still work on macOS after your change. Same for Linux. Same for any future platform.

## What This Means

- Platform-specific code MUST use build tags (`//go:build darwin`, `//go:build linux`, etc.) — not runtime checks that can silently fail
- Every platform-specific file needs a corresponding stub or implementation for other platforms
- Follow the established pattern: `foo_darwin.go` / `foo_linux.go` / `foo_other.go` (see `internal/daemon/service_*.go`, `internal/watcher/filesystem/watcher_*.go`)
- When adding a feature for one platform, verify it doesn't regress another
- When refactoring shared code, mentally trace the code path on EVERY supported platform

## Supported Platforms

| Platform | Status |
|----------|--------|
| macOS ARM/x86 | Full support (primary dev) |
| Linux x86_64 | Supported (daemon + serve) |
| Windows x86_64 | Supported (serve + Windows Services) |

## Before Merging

- Does `go build` succeed with no platform-specific imports leaking across build tags?
- Does `go vet` pass?
- Are all interface implementations complete on every platform?
- Did you check that no platform lost functionality compared to before?
