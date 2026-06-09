# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

Everything CI runs is a Makefile target, so it reproduces locally.

- `make cli` — build `bin/pflex_exporter` (injects `main.version` via ldflags).
- `make test` / `go test ./...` — tests; `make test-race` adds `-race` + coverage.
- Run a single test: `go test ./internal/powerflex/ -run TestGen2SamplesAndLabels` (most logic lives in the `powerflex` package).
- `make ci` — the full gate: gofmt check, `go vet`, `golangci-lint`, `go test -race`, `govulncheck`.
- `make sure` — fmt + vet + test + build + lint (local convenience).
- `make tools` — install pinned `golangci-lint`, `cyclonedx-gomod`, `govulncheck`.
- `make sbom` — CycloneDX SBOM (module-level, via `cyclonedx-gomod`). `make tools-sbom` installs just that tool.
- `make release` / `make release-snapshot` — **GoReleaser** drives the release (`.goreleaser.yaml`): cross-compile + `tar.gz` archives + SBOM + checksums + GitHub Release. `release` needs a `v*` tag and `GITHUB_TOKEN` (CI path); `release-snapshot` is a local dry-run with no publish. The release SBOM stays on `cyclonedx-gomod` (not syft); GoReleaser runs sbom hooks in `./dist`, so the module path is `../`.
- Run it: `./bin/pflex_exporter --config config.yaml [--debug] [--once]`. Cluster secrets are `${ENV_VAR}` references in `config.yaml` (or `passwordFile`); export e.g. `FLEX1_PASSWORD` before running.
- Docs site (MkDocs Material): `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict` (or `serve`).
- `vendor/` is git-ignored; dependencies are managed with `go mod`.

## Architecture

A Go exporter for **Dell PowerFlex (Gen1 and Gen2)** that exposes metrics via **both** a Prometheus `/metrics` endpoint **and** an OTLP metric push, replacing Dell's Python+Telegraf+InfluxDB stack. It mirrors the sibling `nbu_exporter`.

**Snapshot model (the central design choice).** A single background **collection loop** (`internal/powerflex/collector.go`) polls every configured cluster on `collection.interval` and publishes an immutable **snapshot** to a `SnapshotStore` (`snapshot.go`, RWMutex pointer-swap). Both export paths read the latest snapshot rather than fetching on scrape: `PromCollector` (`prometheus.go`, an *unchecked* collector — `Describe` sends nothing so it can emit a dynamic metric-name set) and `OTLPExporter` (`otlp.go`, observable gauges driven by a periodic reader). This decouples PowerFlex API load from the number of scrapers and the OTLP push cadence. `main.go` wires the HTTP server, the loop, hot config reload, and `/health` (snapshot-based).

**Per-cluster client.** One `ClusterClient` per cluster (`client.go`) with bearer-token auth (`auth.go`: `/rest/auth/login` + `/rest/auth/update-token`, 5-min access / 30-min refresh). `config.go`/`safe_config.go` hold a thread-safe config supporting SIGHUP + file-watch reload.

**Generation auto-detection (`gen.go`).** Each cluster's `StoragePool.dataLayout` selects the path: `FineGranularity`/`MediumGranularity` → **Gen1**, `ErasureCoding` → **Gen2**. `collectCluster` branches:
- **Gen1:** `GET /api/instances` + `POST /api/instances/querySelectedStatistics` (embedded `querySelectedStatistics.json`); `deriveSamples` (`derivations.go`) computes iops/bandwidth/io-size/latency from `Bwc` counters.
- **Gen2:** `GET /api/instances` + `POST /dtapi/rest/v1/metrics/query` once per resource type (the v5 mapping table in `statistics_v5.go`); metrics are **pre-computed**, so `deriveSamplesV5` (`derivations_v5.go`) maps them directly. Gen2 adds object types `StorageNode` (renamed SDS), `DeviceGroup` (PMEM/WRC), `Sdt` (NVMe/TCP). The dtapi endpoint takes **one `resource_type` per call** (no multi-type batch), so the per-type queries are issued **concurrently** (`v5QueryConcurrency`, errgroup-bounded) to fit the shared `collection.timeout` instead of serializing ~9 round-trips under it — serial fan-out was what let a slow dtapi exhaust the 8s budget mid-cycle. The request's `metrics` field is a **JSON array** — the form the live dtapi accepts; a comma-separated string returns an instant **HTTP 500** (the PowerFlex 5.0 PDF documents a string, but it is wrong — trusting it shipped a regression in v0.6.2). This matches Dell's reference `siocli`/`sio_sdk` tool.

**Unified sample model (`metrics.go`).** Both generations emit `Sample{Name, []Label, Value}` → `pflex_<obj>_<metric>{op, direction, ...}` plus scalar gauges, with identity/parent labels resolved from the relations graph (`models/relations.go`, built from `/api/instances` link hrefs). Object prefixes live in `metricPrefix`; per-type label builders in `labelBuildersGen1` / `labelBuildersGen2`.

**Optional Kubernetes enrichment (`internal/k8s`, portable).** When `kubernetes.enabled`, a `k8s.Enricher` (client-go) keeps periodically-refreshed maps of PowerFlex `volumeID → {namespace, pvc, pv, storageClass}` (from PVs matching the CSI driver; volume ID parsed from the `<systemID>-<volumeID>` CSI `volumeHandle`) and `sdcIP → nodeName` (from Node addresses). It satisfies the `powerflex.Enricher` interface (`enricher.go`); `collector.go` appends those labels to `Volume`/`Sdc` samples via `appendEnrichment`. It degrades to a **no-op** when no cluster is reachable, so the same binary runs standalone or in-cluster. The enricher is built/refreshed in `main.go` and injected via `WithEnricher`.

**Collector options & robustness (`collector.go`).** `NewCollector` takes functional options: `WithEnricher`, `WithMaxConcurrentClusters` (caps the per-cycle errgroup via `SetLimit`), and `WithDecimation`. **Stats-path fallback:** `collectCluster` tries the generation-selected path, and on a *hard* failure attempts the other path once (`collectGen1`/`collectGen2`); `GetStatisticsV5` now returns an error only when **all** per-type queries fail (partial failures still degrade gracefully).

## Conventions and non-obvious constraints

- **`iops` and bandwidth are already per-second gauges** — in PromQL aggregate with `sum`/`avg`, never `rate()`.
- **Unit-explicit Gen2 names:** Gen2 uses `pflex_<obj>_bandwidth_bytes_per_second`, `_io_size_bytes`, `_latency_microseconds`; Gen1 uses `_bandwidth_kb_per_second`, `_io_size_kb`, `_latency`. `_iops` is shared.
- **Prometheus label-key consistency (load-bearing):** a metric name must carry one label-key set across all series. `Device` and `Volume` are produced by both Gen1 and Gen2 builders, so they emit a **union label set in a fixed canonical order** (`deviceLabels`/`volumeLabels` in `metrics.go`) with empty values for inapplicable keys. Keep the label order identical across builders, or mixed-generation `/metrics` breaks. `TestMixedGenerationMetricsValid` guards this. **The k8s enrichment keys (`namespace`/`persistent_volume_claim`/`persistent_volume`/`storage_class` on Volume; `k8s_node` on SDC) obey the same rule:** when enrichment is enabled they are appended to *every* volume/sdc series (empty when unresolved), since `Enabled()` is process-constant.
- **Decimation is Gen2-only and statistics-only.** `WithDecimation(everyN, slowTypes)` skips those types' v5 queries on non-Nth cycles and reuses the prior snapshot's samples (`reusePriorStatSamples`). Gen1's single `querySelectedStatistics` call can't be partially skipped, so Gen1 is always collected in full. State/health metrics (`_health`/`_info`/`_mapped_sdc`) come from the always-fetched instance list and are excluded from reuse (they stay fresh).
- **resty retry is limited to 429 + transport errors on purpose.** It never retries 4xx (auth/bad-request) **nor 5xx**: PowerFlex 5xx here are deterministic (e.g. a malformed query), so retrying with the 5s backoff under the per-cluster timeout only buries the real status under a `context deadline exceeded` — which is exactly what masked the v0.6.2 `metrics`-format regression. Do not re-add `AddRetryAfterErrorCondition` or a `>= 500` retry clause. (A removed endpoint returns 4xx → no retry → triggers stats-path fallback.)
- Auth is the PowerFlex 4.5+ bearer flow only; no legacy Basic auth.

## Adding metrics or object types

- **Gen1:** add the stat name to `internal/powerflex/querySelectedStatistics.json` — derivation is automatic by suffix (`Bwc`/`Latency`/scalar). A new object type needs a label builder in `metrics.go` + an entry in `labelBuildersGen1` and `metricPrefix`.
- **Gen2:** add an entry to the `v5Metrics` table in `statistics_v5.go` as `(kind, op, direction)`. A new type also needs `v5ResourceType`, a label builder, `labelBuildersGen2`, and `metricPrefix`.
- **Operational state (both gens):** state/health metrics come from a second derivation path in `state.go` (`deriveStateSamples`), driven by *instance properties* rather than the statistics API. Add a state field to `models.Instance`, a `stateLabelsFn`, an `emitState` call, and a severity entry in `healthSeverity`. `pflex_volume_mapped_sdc` is emitted by `volumeMappingSamples` from `Volume.mappedSdcInfo`. Metrics emitted by both generations (Device, SDC, Volume) must reuse the union label builders.

## Testing

`internal/powerflex/client_test.go` runs a mock PowerFlex gateway (`httptest` TLS) that serves `/rest/auth/*`, `/api/instances`, the Gen1 `querySelectedStatistics` endpoint and the Gen2 `/dtapi/rest/v1/metrics/query` endpoint from `testdata/` fixtures (`instances*.json`, `statistics.json`, `statistics-v5.json`). Collector tests assert results via **both** the Prometheus registry gather and an OTLP `ManualReader`. Test HTTP handlers write fixtures through a `writeBytes(io.Writer, …)` helper to avoid the semgrep "write-to-ResponseWriter" rule.

## CI/CD

`.github/workflows/`: `ci.yml` (`make ci` + SBOM artifact + Semgrep), `release.yml` (on `v*` tags: a **GoReleaser** job for binaries/archives + SBOM + a **Homebrew cask** to a GitHub Release, plus a separate multi-arch GHCR image job with SBOM/provenance attestations), `docs.yml` (MkDocs → GitHub Pages via `configure-pages`/`deploy-pages`). The cask publishes to the `fjacquet/homebrew-tap` repo and needs a `HOMEBREW_TAP_GITHUB_TOKEN` secret (cross-repo PAT); it **self-skips** when that secret is empty/absent, so releases don't break before the tap exists. The repo's GitHub Pages `build_type` is `workflow` (Actions deployment), not branch-based. The supply-chain rationale is recorded in `docs/adr/0001-ci-supply-chain-hardening.md`.

**Every action is pinned to a full commit SHA** with a trailing `# vX.Y.Z` comment (not a moving tag). `.github/dependabot.yml` (github-actions + gomod + docker) bumps both the SHA and the comment. When adding or bumping an action: resolve the tag to a SHA with `gh api repos/<owner>/<action>/commits/<tag> --jq .sha` and keep the version comment. Node 20 action runtimes are deprecated (use current Node 24 majors); `astral-sh/setup-uv` has **no** moving `v8` tag (pin an exact version). Verify a tag exists with `gh api repos/<owner>/<action>/releases/latest`.

> A Semgrep scan runs on every file write via a hook and **blocks on findings**. Inline `// nosemgrep` is **not** honored — fix by restructuring (e.g. the `writeBytes(io.Writer, …)` test helper), not suppression. Dockerfiles must declare a non-root `USER`.
