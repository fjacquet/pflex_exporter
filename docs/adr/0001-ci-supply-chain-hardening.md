# ADR 0001 — CI/CD supply-chain hardening: SHA-pinned Actions + GoReleaser release pipeline

- **Status:** Accepted
- **Date:** 2026-06-05
- **Deciders:** Frederic Jacquet

## Context

The release pipeline was hand-rolled: `make release` shelled out a `for`-loop over
`GOOS`/`GOARCH`, called `cyclonedx-gomod` for the SBOM, wrote `checksums.txt`, and
`softprops/action-gh-release` published the GitHub Release. Every GitHub Action across
`ci.yml`, `release.yml`, and `docs.yml` was referenced by a **mutable tag** (`@v6`,
`@v3`, …).

Two supply-chain weaknesses followed from that:

1. **Mutable action tags.** A tag like `actions/checkout@v6` can be silently re-pointed
   to a new commit by the action owner (or an attacker who compromises the repo). The
   workflow would then execute unreviewed code with the workflow's token and secrets.
   This is the [pinning advice in the GitHub hardening guide][gh-harden] and a
   frequently-flagged finding for OSS release pipelines.
2. **Bespoke release shell.** The cross-compile loop, checksum, and SBOM wiring were
   maintained by hand — easy to drift from the container-image path, no archive/manifest
   metadata, and no reproducible-build affordances (`-trimpath`, `mod_timestamp`).

## Decision

### 1. Pin every GitHub Action to a full commit SHA

All `uses:` references are pinned to a 40-character commit SHA with a trailing
`# vX.Y.Z` comment, e.g.:

```yaml
- uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6.0.3
```

A `.github/dependabot.yml` (ecosystems: `github-actions`, `gomod`, `docker`) keeps the
pins current — Dependabot reads the version comment and bumps both the SHA and the
comment, so pinning does not mean stagnation.

### 2. Migrate the release to GoReleaser, keeping cyclonedx-gomod for the SBOM

A `.goreleaser.yaml` (schema v2) now owns cross-compilation
(`linux,darwin × amd64,arm64`), `tar.gz` archives (bundling `LICENSE`, `README.md`,
`config.yaml`), `checksums.txt`, the CycloneDX SBOM, and the GitHub Release (native
auto-generated notes). `release.yml` runs `goreleaser/goreleaser-action`.

The SBOM is **kept on `cyclonedx-gomod`** rather than switching to GoReleaser's default
(syft), via the `sboms` stanza calling `cyclonedx-gomod mod -licenses -json`. This keeps
the SBOM content identical to the CI artifact produced by `make sbom`. GoReleaser runs
sbom hooks inside `./dist`, hence the `../` module path.

The container-image job (multi-arch GHCR push with build-time SBOM + provenance
attestations) is **unchanged** except for SHA-pinning; it remains independent of
GoReleaser. Reproducible-build flags (`-trimpath`, `mod_timestamp`) were added.

### 3. Distribute via a Homebrew cask

GoReleaser's `homebrew_casks` stanza publishes a cask
(`brew install --cask fjacquet/tap/pflex_exporter`; macOS + Linuxbrew) to a separate
`fjacquet/homebrew-tap` repo. A cask (rather than a formula) was chosen per maintainer
preference; the generated cask already covers both macOS and Linux artifacts. A
post-install hook strips the macOS quarantine bit since the binaries are unsigned.
Publishing is **gated on `HOMEBREW_TAP_GITHUB_TOKEN`** (a cross-repo PAT — the default
`GITHUB_TOKEN` cannot push to another repo) and skipped when that secret is absent, so
releases never break before the tap is set up.

## Consequences

**Positive**

- Workflows execute only reviewed, immutable action code; tag-repoint attacks are
  neutralised. Dependabot keeps pins fresh with reviewable PRs.
- A single declarative `.goreleaser.yaml` replaces hand-rolled shell; `make
  release-snapshot` reproduces the full pipeline locally (build + archive + SBOM +
  checksums) without publishing.
- Release assets gain archive metadata, bundled docs/licence, and reproducible builds.

**Negative / trade-offs**

- **Release-asset format changed** from raw binaries (`pflex_exporter_<ver>_<os>_<arch>`)
  to `tar.gz` archives of the same name. Consumers scripting direct binary downloads must
  adjust. (Switchable back via `archives.formats: [binary]` if needed.)
- The injected `main.version` now uses GoReleaser's `{{ .Version }}` (no `v` prefix,
  e.g. `0.5.1`) instead of `git describe` (`v0.5.1`).
- Release CI now depends on GoReleaser and a `cyclonedx-gomod` install step
  (`make tools-sbom`).
- The Homebrew cask needs a one-time setup: an (empty) `fjacquet/homebrew-tap` repo and
  a `HOMEBREW_TAP_GITHUB_TOKEN` repo secret. Until then the cask step self-skips.

## Alternatives considered

- **Keep mutable tags, accept the risk** — rejected; cheap, high-value hardening.
- **GoReleaser default SBOM (syft)** — rejected to keep SBOM content identical to the
  existing `make sbom` artifact and avoid an extra toolchain.
- **GoReleaser-managed container image (ko / dockers_v2)** — deferred; the existing
  `docker/build-push-action` path already produces multi-arch images with attestations.

[gh-harden]: https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions#using-third-party-actions
