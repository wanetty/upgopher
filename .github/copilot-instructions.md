# Copilot Instructions for Upgopher

## Mission
Upgopher is a zero-dependency Go file sharing server with security-first design.

Primary goals when writing code:
- Keep the binary self-contained and dependency-free.
- Preserve or improve security guarantees.
- Keep behavior predictable and backwards compatible.
- Prefer small, testable, modular changes.

## Project Architecture

### Modular Structure
- `upgopher.go`: Main entry point (flags, startup, TLS setup, legacy wiring).
- `server/router.go`: Centralized route registration and auth wrapping.
- `internal/handlers/`: HTTP handlers (`files.go`, `clipboard.go`, `custompath.go`, `ui.go`).
- `internal/security/`: Security primitives (`path.go`, `auth.go`, `ratelimit.go`).
- `utils/`: Pure helper functions (`files.go` for search/formatting).
- `templates/`: HTML builders (`html.go` for row templates).
- `statics/`: Embedded CSS/JS/HTML (`//go:embed`).

### State Management
- Shared state currently lives in `upgopher.go` (`customPaths`, `sharedClipboard`, mutexes).
- Prefer `sync.RWMutex` for read-heavy structures.
- For maps: lock, copy to local variable, unlock, then iterate.
- Keep handlers explicit: pass required state via constructor params.

## Non-Negotiable Security Rules

### 1) Validate Every Filesystem Path
Always call `security.IsSafePath(baseDir, fullPath)` before `os.Open`, `os.Stat`, `os.Remove`, `filepath.Walk`, or similar operations.

```go
fullPath := filepath.Join(baseDir, userInput)
isSafe, err := security.IsSafePath(baseDir, fullPath)
if err != nil || !isSafe {
    http.Error(w, "Bad path", http.StatusForbidden)
    return
}
```

### 2) Encode User Paths in URLs
Any user-originated path in query params must remain base64-encoded.

```go
encodedPath := base64.StdEncoding.EncodeToString([]byte(relativePath))
decodedPath, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
```

### 3) Never Allow Directory Delete via File Delete Endpoints

```go
if fileInfo.IsDir() {
    http.Error(w, "Cannot delete directories", http.StatusForbidden)
    return
}
```

### 4) Keep Auth Wrapper Behavior Intact
Routes must continue to use conditional auth registration in `router.go`.
Auth checks must remain constant-time (`crypto/subtle.ConstantTimeCompare`).

### 5) Preserve Clipboard Rate Limits
Clipboard endpoints should continue to enforce IP-based rate limiting.

### 6) Do Not Leak Sensitive Paths in Errors
Never return absolute or joined filesystem paths in user-facing responses.

## Required Development Workflow

Follow this sequence for every non-trivial change:

1. Understand
- Identify target behavior and impacted modules.
- Trace data flow from route -> handler -> security/util/template.

2. Threat-check
- Identify input vectors (path/query/form/header).
- Verify auth, path safety, and race-safety implications.

3. Implement minimally
- Apply the smallest safe change.
- Reuse existing helpers before adding new ones.
- Avoid broad refactors unless explicitly requested.

4. Test at the right level
- Add/adjust unit tests for changed logic.
- Add security regression tests for new attack surfaces.

5. Validate locally
- Run fast tests first, then race/security checks when relevant.
- Confirm no regressions in expected HTTP status codes and responses.

6. Final review checklist
- No path traversal risk introduced.
- No data races introduced.
- No new dependency added to `go.mod`.
- No user-facing path leakage.
- CLI/help/docs updated when behavior changes.

## Build and Test Commands

```bash
make build          # Compile ./upgopher
make run            # Build + run on :9090
make run-ssl        # Build + run TLS (self-signed)
./upgopher -h       # Check flags/help

make test           # Full test suite
make test-short     # Faster suite for quick iteration
make test-race      # Race detector
make test-coverage  # Coverage report
```

Recommended cadence while coding:
- During iteration: `make test-short`
- Before finalizing concurrency/shared-state changes: `make test-race`
- Before finishing security-sensitive work: `make test`

## Testing Expectations

- Use table-driven tests when possible.
- Use `t.TempDir()` for filesystem isolation.
- Keep tests deterministic; skip long timing tests with `testing.Short()`.
- Security-critical code should include explicit attack-vector tests.
- Coverage target: 60%+ overall, 100% for critical security paths where practical.

## Handler Extension Pattern

When adding endpoints:

1. Add a handler struct in `internal/handlers/` with constructor + `Handle()` method.
2. Register route in `server/router.go` through conditional auth wrapper.
3. Add tests for happy path + misuse/security cases.
4. Ensure response messages are sanitized.

Example pattern:

```go
type MyHandler struct {
    Dir   string
    Quite bool
}

func NewMyHandler(dir string, quite bool) *MyHandler {
    return &MyHandler{Dir: dir, Quite: quite}
}

func (h *MyHandler) Handle() http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // implementation
    }
}
```

## Code Conventions

### Logging

```go
if !quite {
    log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
}
```

### Error Handling
- Return specific but sanitized messages.
- Use proper HTTP status codes.
- Do not include internal paths or implementation details.

### Dependencies
- Keep zero-dependency guarantee (Go standard library only).
- If a dependency seems necessary, treat it as a design discussion first.

## Common Pitfalls to Avoid

1. Missing `IsSafePath` checks before filesystem operations.
2. Iterating shared maps without proper locking/copying.
3. Accidentally changing auth behavior during route registration.
4. Returning filesystem internals in errors/logs sent to clients.
5. Shipping behavior changes without updating README or `-h` output.

## Definition of Done

A change is done only when all are true:
- Code compiles.
- Relevant tests pass.
- Security implications were reviewed.
- No new race conditions or dependency drift.
- Documentation/help output updated if needed.

## Project Facts

- No external dependencies in `go.mod` (std lib only).
- Single self-contained binary with embedded assets.
- Default ports: `9090` (HTTP), `443` with `-ssl`.
- Default upload dir: `./uploads`.
