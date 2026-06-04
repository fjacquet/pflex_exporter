# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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

[0.5.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/fjacquet/pflex_exporter/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/fjacquet/pflex_exporter/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/fjacquet/pflex_exporter/releases/tag/v0.1.0
