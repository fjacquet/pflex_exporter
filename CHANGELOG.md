# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.1] - 2026-06-05

### Added

- The **Homebrew cask is now live** — the `fjacquet/homebrew-tap` repo and
  `HOMEBREW_TAP_GITHUB_TOKEN` secret are configured, so releases publish it:
  `brew install --cask fjacquet/tap/pflex_exporter` (macOS + Linuxbrew).

### Changed

- Trimmed the auto-generated RTK command reference out of `CLAUDE.md` (it lives in the
  global `~/.claude/CLAUDE.md`), leaving only project-specific context.

## [0.6.0] - 2026-06-05

### Changed

- **Release pipeline migrated to GoReleaser** (`.goreleaser.yaml`), replacing the
  hand-rolled `make release` shell loop. It owns cross-compilation
  (`linux,darwin × amd64,arm64`), `tar.gz` archives (bundling `LICENSE`, `README.md`,
  `config.yaml`), `checksums.txt`, the CycloneDX SBOM, and the GitHub Release. The
  module SBOM stays on **cyclonedx-gomod** (not syft), so its content is unchanged.
  Reproducible-build flags (`-trimpath`, `mod_timestamp`) were added.
  **Release assets are now `tar.gz` archives** instead of raw binaries.
- **All GitHub Actions are pinned to full commit SHAs** (with `# vX.Y.Z` comments)
  across `ci.yml`, `release.yml`, and `docs.yml`, hardening against mutable-tag
  repoint attacks.
- Pinned the Dockerfile build stage to `golang:1.26.4`.

### Added

- **Homebrew cask** published to the `fjacquet/homebrew-tap` tap on each release
  (`brew install --cask fjacquet/tap/pflex_exporter`; macOS + Linuxbrew). Skipped
  automatically until the tap repo and `HOMEBREW_TAP_GITHUB_TOKEN` secret exist.
- `.github/dependabot.yml` to keep the SHA-pinned Actions, Go modules, and Docker base
  current (weekly).
- `make tools-sbom` (install just `cyclonedx-gomod`) and `make release-snapshot`
  (local GoReleaser dry-run).
- [ADR 0001](docs/adr/0001-ci-supply-chain-hardening.md) documenting the SHA-pinning
  and GoReleaser migration decisions.

## [0.5.1] - 2026-06-04

### Added

- This changelog.

### Changed

- **k8s enrichment refresh** now lists PersistentVolumes and Nodes concurrently
  (they are independent), with the per-list parsing extracted into focused helpers.
- Deduplicated the PowerFlex CSI driver default into a single exported constant
  (`models.DefaultCSIDriverName`).

_Internal refactor only — no behavior change._

## [0.5.0] - 2026-06-04

### Added

- **Optional Kubernetes workload enrichment.** Volume metrics gain `namespace`,
  `persistent_volume_claim`, `persistent_volume`, and `storage_class` labels; SDC
  metrics gain `k8s_node`. Mapped from the cluster's PersistentVolumes and Nodes via
  the PowerFlex CSI driver. Portable: degrades to a no-op when run standalone.
  Configured under `kubernetes:` (`enabled`, `refreshInterval`, `csiDriverName`,
  `kubeconfig`).
- **Bounded array concurrency** via `collection.maxConcurrentClusters` (0 = unlimited)
  to cap how many clusters are polled in parallel per cycle.
- **Decimation (Gen2)** via `collection.slowResourceEveryN` and
  `collection.slowResourceTypes` to collect slow-changing resource types less often,
  reusing prior samples in between to reduce API load on large arrays.

### Changed

- **Stats-path fallback:** on a hard statistics failure the collector now tries the
  other generation's path once before marking a cluster down. `GetStatisticsV5` returns
  an error only when *all* per-type queries fail (partial failures still degrade
  gracefully).
- Bumped the Go toolchain to **1.26.4**, fixing standard-library advisories
  GO-2026-5037 (`crypto/x509`), GO-2026-5038 (`mime`), and GO-2026-5039
  (`net/textproto`).

## [0.4.0] - 2026-06-02

### Added

- Operational **health/state metrics** (`pflex_<obj>_health`, `pflex_<obj>_info`) derived
  from instance state, with an operational-state severity mapping.
- **Volume-to-SDC correlation** (`pflex_volume_mapped_sdc`) from `mappedSdcInfo`.
- Starter Prometheus **alert rules**, wired into the compose stack.

### Fixed

- Guard the Gen1 capacity ratio against a zero denominator.

## [0.3.0] - 2026-05-29

### Added

- `docker-compose.ghcr.yml` to run the published GHCR image.

### Fixed

- Container log-directory crash; added Grafana to the compose test stack.

## [0.2.1] - 2026-05-28

Maintenance release (CI/packaging).

## [0.2.0] - 2026-05-28

### Added

- **PowerFlex Gen2 (erasure coding)** support alongside Gen1.
- **Kubernetes deployment manifests** (kustomize) and a **systemd** unit with an
  env-file example for host deployment.

### Fixed

- Deploy docs via the GitHub Pages Actions deployment; pin `setup-uv` to v8.1.0.

## [0.1.0] - 2026-05-28

### Added

- Initial release: **PowerFlex Gen1 exporter** exposing metrics via a Prometheus
  `/metrics` endpoint and an OTLP metric push.

[Unreleased]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.1...HEAD
[0.6.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fjacquet/pflex_exporter/releases/tag/v0.1.0
