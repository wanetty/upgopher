# GitHub Copilot Instructions for Upgopher

## Project Overview

Upgopher is a self-contained Go web server for file sharing and management, designed as a portable, cross-platform alternative to Python-based file servers. The entire application is compiled to a single binary with embedded web assets.

## Key Architecture Patterns

### Monolithic Single-File Design
- **Core Logic**: All server functionality resides in `upgopher.go` (~1000+ lines)
- **Embedded Assets**: Static files (HTML/CSS/JS) are embedded using `//go:embed` directives
- **No External Dependencies**: Zero third-party Go modules (see `go.mod`)

### HTTP Handler Pattern
All endpoints follow this security-first pattern:
```go
func customHandler(dir string) http.HandlerFunc {
  return func(w http.ResponseWriter, r *http.Request) {
    // 1. Base64 decode file paths from URL params
    // 2. Validate with isSafePath() to prevent path traversal
    // 3. Check file existence and permissions
    // 4. Apply basic auth wrapper if enabled
  }
}
```

### Path Security Architecture
**Critical**: All file paths are base64-encoded in URLs and must be validated:
```go
// Encoding for URL safety
encodedPath := base64.StdEncoding.EncodeToString([]byte(filePath))

// Always validate before file operations  
isSafe, err := isSafePath(dir, fullPath)
if err != nil || !isSafe {
  http.Error(w, "Bad path", http.StatusForbidden)
  return
}
```

## Critical Development Workflows

### Build System
**GoReleaser-based**: Cross-platform releases managed via `.goreleaser.yml`
```bash
# Local development build
go build

# Cross-platform release build (requires tag)
goreleaser release --snapshot --clean

# GitHub Actions auto-builds on tag push
```

### Asset Embedding System
Frontend changes require understanding the embedding pattern in `internal/statics/`:
- `statics.go` uses `//go:embed templates css js` to bundle assets
- Template injection via `template.CSS`, `template.HTML`, `template.JS` types
- **No build step needed** - assets compile directly into binary

### Authentication Wrapper Pattern
When adding endpoints, check if auth is enabled:
```go
// In main(), conditionally wrap with authentication
if *user != "" && *pass != "" {
  http.HandleFunc("/new-endpoint", applyBasicAuth(newHandler, *user, *pass))
} else {
  http.HandleFunc("/new-endpoint", newHandler)
}
```

## Essential Code Patterns

### File Operations
```go
// Standard file serving pattern with safety checks
func fileHandler(dir string) http.HandlerFunc {
  // Always decode base64 paths from ?path= param
  // Always call isSafePath() before file ops
  // Use filepath.Join() for cross-platform paths
  // Check os.Stat() before serving
}
```

### Global State Management
Key globals that affect behavior:
- `quite bool` - controls logging output
- `showHiddenFiles bool` - runtime toggle for hidden files
- `disableHiddenFiles bool` - compile-time disable flag
- `customPaths map[string]string` - file shortcuts/aliases

### Logging Convention
Structured logging with timestamp and HTTP details:
```go
if !quite {
  log.Printf("[%s] [%s - %s] %s %s\n", 
    time.Now().Format("2006-01-02 15:04:05"), 
    r.Method, statusCode, r.URL.Path, r.RemoteAddr)
}
```

## Integration Points

### Adding New Endpoints
1. Create handler function following security pattern
2. Register in `main()` with optional auth wrapper
3. Update frontend if UI changes needed
4. Test path traversal protection

### Frontend Modifications  
- **HTML**: Edit `internal/statics/templates/index.html`
- **CSS**: Edit `internal/statics/css/styles.css` 
- **JS**: Edit `internal/statics/js/main.js`
- **Icons**: FontAwesome 4.7.0 CDN used throughout

### Release Process
- Tags trigger GitHub Actions workflow in `.github/workflows/go.yml`
- GoReleaser builds for multiple OS/arch combinations
- Archives include LICENSE and README.md
- Self-signed SSL cert generation built-in for HTTPS mode
