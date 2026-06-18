# AGENTS.md

Compact, repo-specific notes for OpenCode sessions.

## Non-negotiables
- Standard library only; avoid new deps in `go.mod`.
- Keep the single self-contained binary model: no required external files or runtime assets.
- Must remain runnable as a standalone binary across major OSes (Windows/macOS/Linux).
- Always `security.IsSafePath(baseDir, fullPath)` before any filesystem read/write/delete/walk.
- User paths in URLs must remain base64-encoded (decode from `?path=` only).
- Keep auth wrapping behavior in `internal/server/router.go` intact (conditional BasicAuth, constant-time compare).
- Do not leak absolute filesystem paths in user-facing errors.

## Entry points & wiring
- Main entry: `upgopher.go` (flags, TLS, shared state, calls `internal/server.SetupRoutes`).
- Routes: `internal/server/router.go` (central registration + auth wrapping).
- Handlers: `internal/handlers/` (files, clipboard, custom path, UI).
- Security: `internal/security/` (path safety, auth, rate limit).
- Embedded assets: `static/` + `//go:embed`.

## Build/test/dev commands (Makefile)
- `make build` — compila `./upgopher` con `-ldflags="-s -w"`.
- `make run`, `make run-auth` (basic auth), `make run-ssl` (HTTPS auto-firmado).
- `make test`, `make test-short`, `make test-race`, `make test-coverage`.
- `make lint` — `go vet`.
- `make clean` — elimina binarios y `coverage.out`.
- `make dev` — `test-race` + `lint` (check rápido pre-commit).
- `make ci` — `test-race` + `lint` + `build` (pipeline completo).

## State & concurrency
- Shared maps live in `upgopher.go` (e.g., `customPaths`); lock with `sync.RWMutex`, copy then iterate.

## Docs/instructions to honor
- `README.md` for CLI flags and usage examples.
- `.github/copilot-instructions.md` has stricter security and workflow expectations; treat as authoritative.
