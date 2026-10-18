# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-10-19

### Added
- Go CLI tool `sitecheck` — single-pass availability check for a list of URLs
  via HTTP GET, with DNS resolution and TLS certificate expiration detection.
- Flags: `--concurrency`, `--retries`, `--json`, `--json-report`, `--output`,
  `--text-output`, `--version`.
- System info block in reports: hostname, username, OS name
  (Linux / macOS / Windows).
- Network info block in reports: local IP, outgoing IP, gateway (with ping),
  DNS servers (resolved / resolv.conf / macOS / Windows), VPN interfaces,
  all local IPv4 addresses.
- Cross-platform gateway ping dispatcher (`pingGateway`) with per-OS
  implementations.
- Cross-compile targets: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64,
  windows/amd64.
- `Makefile` with targets: `build`, `test`, `vet`, `lint`, `fmt`, `fmt-check`,
  `tidy`, `clean`, `cross`, `help`.
- GitHub Actions CI: check (fmt, vet, golangci-lint, test -race, build),
  cross-compile matrix, release on `v*` tags.
- `.golangci.yml` (v2) with linters: govet, errcheck, staticcheck, unused,
  gosimple, gofmt, goimports, gosec, revive, misspell, unconvert, unparam,
  prealloc, ineffassign.
- Retryable error classification: `isRetryableError()` in `errors.go` —
  DeadlineExceeded → retry, TLS → no retry, DNS → `IsTemporary`,
  connection refused → retry, generic → retry.
- 200 ms backoff between retry attempts (fixed, see `app.go`).
- `--version` prints `sitecheck <version> (commit <hash>, <date>, <os>/<arch>)`.
- `TODOS.md` — prioritized task list for future work.

### Changed
- Replaced Python implementation with a Go implementation.
- `go.mod` moved to repository root; source files in `sitecheck/`.
- Code split from a single 864-line `main.go` into 14 focused files, all
  `package main`.
- CI: `golangci-lint-action` replaces `make lint` in GitHub Actions;
  linter version pinned with `v` prefix.
- `golangci-lint` configuration migrated to v2 schema.

### Removed
- `checker-go/` (archived Go prototype).
- `checker.py` (original Python implementation).

### Fixed
- DNS / TLS are not attempted when the HTTP request already failed
  (context cancellation, invalid URL).
- `isConnectionRefused` — Linux-only via `syscall.ECONNREFUSED` (see
  `TODOS.md` p. 3 for the macOS/Windows fix).
