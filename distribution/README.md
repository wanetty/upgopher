# Distribution repositories and secrets

Upgopher publishes release metadata to two project-owned repositories:

- `wanetty/homebrew-tap`
- `wanetty/scoop-bucket`

Create both as public repositories with a `main` branch before pushing the
v1.20.0 tag. The release workflow writes generated files directly to that
branch.

## Required GitHub Actions secrets

Configure these in `wanetty/upgopher`:

| Secret | Scope |
| --- | --- |
| `DISTRIBUTION_GITHUB_TOKEN` | Fine-grained token with repository Contents read/write for `homebrew-tap` and `scoop-bucket` only |
| `DOCKERHUB_USERNAME` | Docker Hub account name |
| `DOCKERHUB_TOKEN` | Docker Hub access token with permission to update `wanetty/upgopher` |

The built-in `GITHUB_TOKEN` publishes release assets and the GHCR image. Do not
reuse a personal password or a broadly scoped classic token.

Initialize `homebrew-tap` with a short README and initialize `scoop-bucket` with
a short README. GoReleaser creates the Cask and Scoop manifest on the first
tagged release. The release workflow then adds Scoop `checkver` and
`autoupdate` metadata while preserving GoReleaser's architecture-specific
SHA-256 hashes.

## GitHub Pages

In repository settings, select **GitHub Actions** as the Pages source. The
`Pages` workflow publishes the contents of `/docs` on changes to `main`.
