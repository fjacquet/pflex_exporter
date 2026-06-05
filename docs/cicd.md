# CI/CD & SBOM

Every check CI runs is a Makefile target, so it reproduces locally.

## CI (`.github/workflows/ci.yml`)

Runs on push to `main` and on pull requests:

- **quality** — `make ci`: `gofmt` check, `go vet`, `golangci-lint`, `go test -race`
  with coverage, and `govulncheck`. Coverage is uploaded as an artifact.
- **sbom** — `make sbom`: a CycloneDX SBOM uploaded as an artifact.
- **semgrep** — a static security scan (`semgrep scan --config auto --error`).

```bash
make tools   # install pinned golangci-lint, cyclonedx-gomod, govulncheck
make ci       # the same gate CI runs
```

## Release (`.github/workflows/release.yml`)

Runs on `v*` tags:

- **goreleaser** — [GoReleaser](https://goreleaser.com) (`.goreleaser.yaml`)
  cross-compiles `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`, builds `tar.gz`
  archives (bundling `LICENSE`, `README.md`, `config.yaml`), generates the CycloneDX
  SBOM, writes `checksums.txt`, and publishes a GitHub Release with native auto-generated
  notes and all artifacts attached.
- **image** — builds and pushes a multi-arch container image to
  `ghcr.io/<owner>/pflex_exporter` with build-time **SBOM and provenance attestations**.

```bash
git tag v0.1.0 && git push origin v0.1.0   # triggers the release
make release-snapshot                       # local dry-run (no publish)
```

## SBOM

Two SBOMs are produced:

- **Module SBOM** — `cyclonedx-gomod` emits a CycloneDX SBOM describing the Go
  dependency tree. `make sbom` writes `dist/sbom.cdx.json` (uploaded by CI); at release
  time GoReleaser's `sboms` stanza runs the **same tool** and attaches the SBOM to the
  GitHub Release.
- **Image SBOM** — `docker/build-push-action` attaches an SBOM and provenance
  attestation to the pushed container image.

## Homebrew

GoReleaser publishes a Homebrew **cask** to the `fjacquet/homebrew-tap` tap on each
release (macOS + Linuxbrew):

```bash
brew install --cask fjacquet/tap/pflex_exporter
```

This requires two one-time prerequisites; until they exist the cask step is **skipped**
automatically (releases still succeed):

1. An (empty) `github.com/fjacquet/homebrew-tap` repository.
2. A repo secret `HOMEBREW_TAP_GITHUB_TOKEN` — a PAT with write access to the tap repo.
   The default `GITHUB_TOKEN` cannot push to a different repository.

## Action pinning

Every GitHub Action is pinned to a full commit SHA with a `# vX.Y.Z` comment (e.g.
`actions/checkout@df4cb1c… # v6.0.3`) to defend against mutable-tag repoint attacks.
`.github/dependabot.yml` keeps the pins (plus Go modules and the Docker base) current —
Dependabot reads the version comment and bumps both. See
[ADR 0001](adr/0001-ci-supply-chain-hardening.md) for the rationale.

## Versioning

The build version is injected via `-ldflags "-X main.version=$(VERSION)"`, where
`VERSION` defaults to `git describe --tags`. Check it with:

```bash
pflex_exporter --version
```

## Documentation

This site is built with MkDocs Material and deployed to GitHub Pages by
`.github/workflows/docs.yml` on every push to `main`.

```bash
uvx --with mkdocs-material --with pymdown-extensions mkdocs serve   # preview locally
```
