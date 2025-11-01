# Copilot Instructions for Upgopher

## Project Overview
Upgopher is a zero-dependency Go web server for file sharing with security-first design. Single binary distribution with embedded assets.

## Architecture

### Modular Structure (Post-Refactor)
- **`upgopher.go`**: Main entry point (~200 lines) - handles CLI args, TLS setup, and legacy handler delegation
- **`internal/server/router.go`**: Centralized route registration with conditional auth wrapping
- **`internal/handlers/`**: HTTP handlers (`files.go`, `clipboard.go`, `custompath.go`, `ui.go`)
- **`internal/security/`**: Security primitives (`path.go`, `auth.go`, `ratelimit.go`)
- **`internal/utils/`**: Pure functions (`files.go` for search and formatting)
- **`internal/templates/`**: HTML generation (`html.go` for row templates)
- **`internal/statics/`**: Embedded CSS/JS/HTML with `//go:embed`

### State Management
- **Global state in `upgopher.go`**: `customPaths` map, `sharedClipboard` string, mutexes passed to handlers
- **Thread-safety pattern**: Use `sync.RWMutex` for read-heavy maps (e.g., `customPathsMutex.RLock()` → copy map → `RUnlock()`)
- **No app struct**: State passed via function parameters to handler constructors

## Critical Security Patterns

### 1. Path Traversal Prevention (ALWAYS USE)
```go
import "github.com/wanetty/upgopher/internal/security"

fullPath := filepath.Join(baseDir, userInput)
isSafe, err := security.IsSafePath(baseDir, fullPath)
if err != nil || !isSafe {
    http.Error(w, "Bad path", http.StatusForbidden)
    return
}
```
**Used in**: All file operations (`files.go`, `custompath.go`, `upgopher.go` legacy handlers)

### 2. Base64 Path Encoding
User-provided paths are **always base64-encoded** in URLs:
```go
// Encoding (when generating links)
encodedPath := base64.StdEncoding.EncodeToString([]byte(relativePath))

// Decoding (in handlers)
decodedPath, err := base64.StdEncoding.DecodeString(r.URL.Query().Get("path"))
```

### 3. Directory Deletion Protection
In delete handlers, **always verify it's a file**:
```go
if fileInfo.IsDir() {
    http.Error(w, "Cannot delete directories", http.StatusForbidden)
    return
}
```

### 4. Authentication Wrapper
Routes use conditional auth in `server/router.go`:
```go
func registerRoute(pattern string, handler http.Handler, user, pass string) {
    if user != "" && pass != "" {
        http.Handle(pattern, security.ApplyBasicAuth(convertToHandlerFunc(handler), user, pass))
    } else {
        http.Handle(pattern, handler)
    }
}
```
**Auth uses constant-time comparison** (`crypto/subtle.ConstantTimeCompare`) in `security/auth.go`.

### 5. Rate Limiting
Clipboard endpoint uses IP-based rate limiting (20 req/min):
```go
import "github.com/wanetty/upgopher/internal/security"

if !security.CheckRateLimit(extractIP(r)) {
    http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

## Development Workflows

### Building & Running
```bash
make build      # Compiles to ./upgopher
make run        # Build + run on :9090
make run-ssl    # Build + run with self-signed cert
./upgopher -h   # See all CLI flags
```

### Testing Strategy
- **Unit tests** (`upgopher_test.go`): Core functions like `SearchInFile`, `FormatFileSize`
- **Security tests** (`security_test.go`): Attack vectors (path traversal, race conditions, auth bypass)
- **Run tests**:
  ```bash
  make test           # All tests
  make test-race      # With race detector (catches mutex issues)
  make test-coverage  # Generate coverage.out
  make test-short     # Skip time-based tests (65s rate limit recovery)
  ```
- **Coverage goal**: 60%+ overall, 100% for security-critical functions

### Adding New Handlers
1. Create handler in `internal/handlers/` with struct + constructor pattern:
   ```go
   type MyHandler struct {
       Dir   string
       Quite bool
   }
   func NewMyHandler(dir string, quite bool) *MyHandler { /* ... */ }
   func (h *MyHandler) Handle() http.HandlerFunc { /* ... */ }
   ```
2. Register in `internal/server/router.go`:
   ```go
   myHandler := handlers.NewMyHandler(dir, quite)
   registerRoute("/my-endpoint", myHandler.Handle(), user, pass)
   ```
3. Add security tests in `security_test.go` for attack vectors

## Code Conventions

### Logging Pattern
```go
if !quite {
    log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
}
```
**Used in**: All handlers for request/response logging

### Error Responses (NO PATH LEAKAGE)
❌ **Never expose filesystem paths**:
```go
http.Error(w, fmt.Sprintf("File not found: %s", fullPath), 404) // BAD
```
✅ **Use sanitized messages**:
```go
http.Error(w, "File not found", http.StatusNotFound) // GOOD
```

### Embedded Assets
Static files use `//go:embed` in `upgopher.go` and `internal/statics/statics.go`:
```go
//go:embed static/favicon.ico
var favicon embed.FS

// Serve with:
http.FileServer(http.FS(favicon))
```

## Common Pitfalls

1. **Forgetting `IsSafePath` validation**: Always call before `os.Open`, `os.Stat`, `filepath.Walk`
2. **Race conditions on shared maps**: Use `RWMutex` or copy map under lock (see `files.go` `createTable`)
3. **Testing time-based logic**: Use `testing.Short()` to skip long tests:
   ```go
   if testing.Short() {
       t.Skip("Skipping time-based test")
   }
   ```
4. **Breaking zero-dependency guarantee**: Only use Go standard library (check `go.mod`)

## Project Context

- **No external dependencies**: `go.mod` only declares `go 1.19`
- **Single binary**: All assets embedded, no external files needed
- **Default port**: 9090 (HTTP), 443 (HTTPS with `-ssl`)
- **Default upload dir**: `./uploads`
- **Branch**: `refactor/modular-architecture` (post-refactor from monolithic structure)

## When Modifying Code

- **Security functions**: Add attack vector test in `security_test.go`
- **File operations**: Use `t.TempDir()` in tests for isolated filesystem
- **Concurrency**: Run `make test-race` to catch data races
- **UI changes**: Update `internal/statics/templates/index.html` or `internal/templates/html.go`
- **Breaking changes**: Update README.md examples and `-h` flag descriptions
