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

- **binaries** — `make release` cross-compiles `linux/{amd64,arm64}` and
  `darwin/{amd64,arm64}`, generates the SBOM, writes `checksums.txt`, and publishes a
  GitHub Release with all artifacts attached.
- **image** — builds and pushes a multi-arch container image to
  `ghcr.io/<owner>/pflex_exporter` with build-time **SBOM and provenance attestations**.

```bash
git tag v0.1.0 && git push origin v0.1.0
```

## SBOM

Two SBOMs are produced:

- **Module SBOM** — `make sbom` runs `cyclonedx-gomod` to emit `dist/sbom.cdx.json`
  (CycloneDX) describing the Go dependency tree; attached to each release.
- **Image SBOM** — `docker/build-push-action` attaches an SBOM and provenance
  attestation to the pushed container image.

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
