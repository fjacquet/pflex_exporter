# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.6.4] - 2026-06-09

### Added

- **Debug visibility for Gen2 v5 collection.** At `--debug`, each dtapi metrics query logs
  its resource type, resource count, and elapsed time
  (`cluster "flex-v3": v5 query volume -> 412 resources in 1.2s`), followed by a per-cluster
  summary (`v5 stats 9/9 types ok in …`). Failed queries now report how long they took, which
  distinguishes a fast server reject (HTTP 500) from a slow timeout — the ambiguity that
  masked the dtapi failures. Silent at the default log level.

## [0.6.3] - 2026-06-09

### Fixed

- **Gen2 metrics restored — fixes a v0.6.2 regression.** v0.6.2 sent the dtapi `metrics`
  field as a comma-separated string (per the PowerFlex 5.0 PDF schema), but the live
  `/dtapi/rest/v1/metrics/query` rejects that with an instant **HTTP 500** and accepts only a
  **JSON array** — as Dell's reference `siocli`/`sio_sdk` tool does. Reverted `metrics` to a
  JSON array, so Gen2 statistics collection works again.
- **5xx responses are no longer retried.** resty was retrying HTTP 500 with a 5s backoff
  under the per-cluster timeout, turning an instant server error into a misleading
  `context deadline exceeded` and hiding the real status (this masked the regression above).
  Only `429` and transport errors are retried now; 4xx/5xx surface immediately with their
  status and body.

## [0.6.2] - 2026-06-09

### Fixed

- **Gen2 statistics no longer time out on slower clusters.** The v5 dtapi metric queries
  (one per resource type) now run **concurrently** instead of serially, so the ~9
  round-trips fit the shared per-cluster `collection.timeout` (default 8s) the way Gen1's
  single call always has. Previously a slow `/dtapi/rest/v1/metrics/query` endpoint
  exhausted the budget mid-cycle, producing recurring `context deadline exceeded` warnings
  and missing Gen2 metrics. The request's `metrics` field is now also sent as the
  documented comma-separated string rather than a JSON array.

### Changed

- CI GitHub Actions bumped via Dependabot; Homebrew install docs corrected to note casks
  are macOS-only.

## [0.6.1] - 2026-06-05

### Added

- The **Homebrew cask is now live** — the `fjacquet/homebrew-tap` repo and
  `HOMEBREW_TAP_GITHUB_TOKEN` secret are configured, so releases publish it:
  `brew install --cask fjacquet/tap/pflex_exporter` (macOS; casks are not supported on
  Linux — use the release archive, the GHCR image, or `go install` there).

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
  (`brew install --cask fjacquet/tap/pflex_exporter`; macOS only). Skipped
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

[Unreleased]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.4...HEAD
[0.6.4]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.3...v0.6.4
[0.6.3]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.2...v0.6.3
[0.6.2]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fjacquet/pflex_exporter/releases/tag/v0.1.0
