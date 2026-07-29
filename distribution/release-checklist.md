# v1.20.0 release checklist

## Before tagging

- [ ] `make test`, `make test-race`, `make lint`, and `make build` pass.
- [ ] `goreleaser check` passes with the current GoReleaser v2.
- [ ] `scripts/check-community-scripts-contract.sh` passes against a release
      snapshot, preserving the Linux `amd64`/`arm64` archive names and embedded
      `upgopher` path consumed by the Proxmox Community Script.
- [ ] All links in README and `/docs` resolve.
- [ ] The website is checked at desktop and mobile widths with keyboard-only
      navigation and reduced motion.
- [ ] `docs/assets/upgopher-demo.gif` is below 10 MB and contains no private
      data or paths.
- [ ] `wanetty/homebrew-tap` and `wanetty/scoop-bucket` exist and the
      distribution token can write only to those repositories.
- [ ] Docker Hub contains the public `wanetty/upgopher` repository and its token
      is configured.
- [ ] GitHub Pages uses GitHub Actions as its source.
- [ ] Copy `.github/release-notes/v1.20.0.md` into the release draft body.

## Create the draft

```bash
git tag -a v1.20.0 -m "Upgopher v1.20.0"
git push origin v1.20.0
```

Do not publish the draft until both release workflows finish successfully.

## Verify artifacts

- [ ] Linux, macOS, and Windows archives exist for each configured architecture.
- [ ] `checksums.txt` matches downloaded sample archives.
- [ ] Extracted binaries run with `-h`.
- [ ] An existing Community Scripts LXC updates to v1.20.0 without changing
      `/opt/upgopher/uploads` or `/etc/systemd/system/upgopher.service`.
- [ ] Homebrew installs and uninstalls on macOS arm64 and amd64.
- [ ] Scoop installs, updates, and uninstalls on Windows amd64 and arm64.
- [ ] Both container registries expose `1.20.0`, `1.20`, `1`, and `latest`.
- [ ] `linux/amd64` and `linux/arm64` images start as UID/GID `65532`.
- [ ] Container file upload, read-only mode, Basic Auth, custom TLS, and ZIP
      download work with a bind-mounted `/data`.
- [ ] The website and README installation commands point to public artifacts.

## Publish and follow up

- [ ] Publish the GitHub release.
- [ ] Submit the prepared awesome-selfhosted entry.
- [ ] Record the publication timestamp in `adoption-baseline.md`.
- [ ] Record the same metrics after 7 and 30 days.
