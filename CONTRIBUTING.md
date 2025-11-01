# Contributing to Upgopher

Thank you for your interest in contributing to Upgopher! This document provides guidelines and information about the project's architecture.

## Project Architecture

Upgopher has been refactored from a monolithic structure to a modular architecture while maintaining **zero external dependencies** and **single binary distribution**.

### Directory Structure

```
upgopher/
├── upgopher.go              # Main entry point (~200 lines)
├── upgopher_test.go         # Unit tests
├── security_test.go         # Security tests
├── Makefile                 # Development tasks
├── go.mod                   # Go module (zero dependencies)
├── internal/
│   ├── security/            # Security functions
│   │   ├── path.go         # Path traversal prevention
│   │   ├── ratelimit.go    # Rate limiting
│   │   └── auth.go         # HTTP Basic Auth
│   ├── utils/              # Utility functions
│   │   └── files.go        # File operations (search, format size)
│   ├── templates/          # HTML generation
│   │   └── html.go         # File/folder row templates
│   ├── app/                # Application state
│   │   └── app.go          # App struct with config and state
│   ├── handlers/           # HTTP handlers (future)
│   ├── server/             # Routing (future)
│   └── statics/            # Embedded assets
│       ├── statics.go      # Asset embedding
│       ├── templates/      # HTML templates
│       ├── css/            # Stylesheets
│       └── js/             # JavaScript
├── static/                 # Static files (favicon, logo)
└── uploads/                # Default upload directory
```

### Key Design Principles

1. **Zero Dependencies**: Only Go standard library
2. **Single Binary**: All assets embedded with `//go:embed`
3. **Thread-Safe**: All shared state protected by mutexes
4. **Security First**: Path traversal prevention, rate limiting, constant-time auth
5. **Testable**: >60% code coverage with comprehensive security tests

### Package Descriptions

#### `internal/security`
- **`path.go`**: `IsSafePath(baseDir, userPath)` - Prevents directory traversal attacks
- **`ratelimit.go`**: `CheckRateLimit(ip)` - 20 req/min per IP for clipboard endpoint
- **`auth.go`**: `ApplyBasicAuth(handler, user, pass)` - HTTP Basic Authentication wrapper

#### `internal/utils`
- **`files.go`**: 
  - `FormatFileSize(size int64)` - Human-readable file sizes
  - `SearchInFile(path, term, caseSensitive, wholeWord)` - Search within text files
  - `SearchResult` type for search results

#### `internal/templates`
- **`html.go`**:
  - `CreateFileRow()` - Generate HTML for file entries
  - `CreateFolderRow()` - Generate HTML for folder entries
  - `CreateBackButton()`, `CreateZipButton()` - UI buttons
  - `IsTextFile()` - Determine if file is readable as text

#### `internal/app`
- **`app.go`**:
  - `Config` struct - Server configuration
  - `App` struct - Application state with thread-safe methods
  - Methods: `GetCustomPath()`, `SetClipboard()`, `ToggleHiddenFiles()`, etc.

## Development Workflow

### Prerequisites
- Go 1.19 or higher
- Make (optional, but recommended)

### Setup
```bash
git clone https://github.com/wanetty/upgopher.git
cd upgopher
go build
```

### Available Make Targets
```bash
make help           # Show available commands
make build          # Compile the project
make test           # Run all tests
make test-race      # Run tests with race detector
make test-coverage  # Generate coverage report
make lint           # Run static analysis
make clean          # Remove binaries
make run            # Build and run server
make ci             # Full CI pipeline
```

### Running Tests
```bash
# All tests
make test

# With race detection
make test-race

# Only fast tests (skip 65s rate limit test)
go test -v -short ./...

# Specific test
go test -v -run TestIsSafePath
```

### Code Style
- Follow standard Go formatting: `go fmt ./...`
- Run `go vet ./...` before committing
- Add tests for new features
- Security-critical code requires 100% test coverage

## Making Changes

### Adding New Features
1. **Create a feature branch**: `git checkout -b feature/your-feature`
2. **Write tests first**: TDD approach preferred
3. **Implement the feature**: Keep functions small and focused
4. **Update documentation**: README.md and code comments
5. **Run full test suite**: `make ci`
6. **Submit PR**: With clear description and test coverage

### Security Guidelines
- **Always use `security.IsSafePath()`** before file operations
- **Never trust user input**: Validate and sanitize all inputs
- **Use constant-time comparisons** for authentication (already in `security.ApplyBasicAuth`)
- **Add security tests**: Test attack vectors in `security_test.go`

### Modifying Existing Code
1. **Check test coverage**: `make test-coverage`
2. **Update affected tests**: Ensure they still pass
3. **Run race detector**: `make test-race`
4. **Test manually**: Build and run the server

## Testing Guidelines

### Test Categories
- **Unit Tests** (`upgopher_test.go`): Core functionality
- **Security Tests** (`security_test.go`): Attack vector validation
- **Integration Tests** (`internal/integration/`): End-to-end flows (future)

### Writing Tests
```go
func TestYourFeature(t *testing.T) {
    // Use t.TempDir() for temporary directories
    tempDir := t.TempDir()
    
    // Table-driven tests preferred
    tests := []struct {
        name string
        input string
        want string
    }{
        {"case 1", "input", "expected"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := YourFunction(tt.input)
            if got != tt.want {
                t.Errorf("got %v, want %v", got, tt.want)
            }
        })
    }
}
```

### Security Test Example
```go
func TestYourSecurityFeature(t *testing.T) {
    attacks := []string{
        "../../../etc/passwd",
        "..%2F..%2F..%2Fetc%2Fpasswd",
        "/etc/passwd",
    }
    
    for _, attack := range attacks {
        if !isBlocked(attack) {
            t.Errorf("Attack not blocked: %s", attack)
        }
    }
}
```

## Pull Request Process

1. **Update tests**: Ensure all tests pass
2. **Update documentation**: README.md if user-facing changes
3. **Add changelog entry**: Update README.md changelog section
4. **Run full CI**: `make ci` must pass
5. **Request review**: Tag maintainers if needed
6. **Address feedback**: Be responsive to review comments

## Release Process

Releases are automated via GoReleaser when a tag is pushed:

```bash
# Tag a new version
git tag -a v1.12.0 -m "Release v1.12.0"
git push origin v1.12.0

# GitHub Actions will:
# - Run tests
# - Build binaries for all platforms
# - Create GitHub release
# - Attach binaries
```

### Version Numbering
- **Major** (v2.0.0): Breaking changes
- **Minor** (v1.12.0): New features, backward compatible
- **Patch** (v1.11.1): Bug fixes only

## Code Review Checklist

Before submitting a PR, verify:
- [ ] All tests pass (`make test`)
- [ ] No race conditions (`make test-race`)
- [ ] Code is formatted (`go fmt`)
- [ ] No lint errors (`make lint`)
- [ ] Documentation updated
- [ ] Changelog entry added
- [ ] Backward compatible (or breaking change documented)
- [ ] Security implications considered
- [ ] Single binary still works (`make build && ./upgopher -h`)

## Getting Help

- **Questions**: Open a GitHub issue with `question` label
- **Bug Reports**: Open an issue with steps to reproduce
- **Security Issues**: Contact [@gm_eduard](https://twitter.com/gm_eduard/) privately
- **Feature Requests**: Open an issue with `enhancement` label

## Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help others learn and grow
- Prioritize security and user safety

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
