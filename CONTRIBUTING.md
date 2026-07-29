# Contributing to Upgopher

Thank you for helping improve Upgopher. The project favors small, reviewable
changes that preserve its security guarantees and self-contained binary.

## Development setup

Requirements:

- Go 1.19 or newer
- Make (optional)

```bash
git clone https://github.com/wanetty/upgopher.git
cd upgopher
make build
make test-short
```

The main development commands are:

```bash
make build          # Build ./upgopher
make test           # Run the full test suite
make test-short     # Skip long-running tests
make test-race      # Run tests with the race detector
make test-coverage  # Write coverage.out and print coverage
make lint           # Run go vet
make dev            # Race detector and vet
make ci             # Race detector, vet, and production build
```

## Architecture

- `upgopher.go` parses flags, creates shared state, and starts HTTP or HTTPS.
- `internal/server/router.go` registers routes and applies optional Basic Auth.
- `internal/handlers/` contains file, clipboard, custom-path, and UI handlers.
- `internal/security/` contains path validation, auth, and rate limiting.
- `internal/statics/`, `static/`, and `//go:embed` keep runtime assets inside the
  executable.
- `docs/` is the standalone GitHub Pages site and is not required at runtime.

## Non-negotiable constraints

- Use only the Go standard library; do not add modules to `go.mod`.
- Keep all runtime assets embedded in the executable.
- Preserve Windows, macOS, and Linux support.
- Call `security.IsSafePath(baseDir, fullPath)` before every filesystem read,
  write, delete, or walk involving user-controlled paths.
- Keep user paths base64-encoded in URL query parameters.
- Preserve conditional Basic Auth registration and constant-time comparison.
- Do not expose absolute host paths in user-facing errors.
- Protect shared maps with their mutex; copy under lock before iterating when
  practical.

Read [AGENTS.md](./AGENTS.md) and
[the repository instructions](./.github/copilot-instructions.md) before making
code changes.

## Pull requests

1. Create a focused branch.
2. Add or update tests for changed behavior.
3. Run `gofmt` on changed Go files.
4. Run `make dev`; run `make ci` before requesting review.
5. Update the README, CLI help, or website when user-facing behavior changes.
6. Explain the behavior, security implications, and validation in the PR.

Use table-driven tests where useful and `t.TempDir()` for filesystem isolation.
Security-sensitive behavior should include explicit misuse or traversal cases.

## Documentation and website

The public website is plain HTML, CSS, and JavaScript under `docs/`. It must
remain usable without a build step or third-party runtime dependency.

To refresh the animated product demo after replacing its source screenshots:

```bash
go run ./scripts/create-demo-gif.go
```

Check links, keyboard navigation, narrow viewports, reduced-motion behavior, and
image sizes before submitting documentation changes.

## Release process

Pushing a `v*` tag starts independent GitHub Actions workflows:

- GoReleaser builds archives and checksums, updates the project Homebrew tap and
  Scoop bucket, and creates a draft GitHub release.
- The container workflow publishes multi-architecture images to GHCR and Docker
  Hub.

The draft is published manually only after following
[`distribution/release-checklist.md`](./distribution/release-checklist.md).
Maintainers must never put distribution tokens in the repository or logs.

## Security reports

Do not open public issues for suspected vulnerabilities. Follow
[SECURITY.md](./SECURITY.md) to coordinate a private report.

Contributions are licensed under the [MIT License](./LICENSE).
