# pflex_exporter

[![CI](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml)
[![Release](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/pflex_exporter)](https://goreportcard.com/report/github.com/fjacquet/pflex_exporter)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fjacquet/pflex_exporter)](go.mod)
[![Latest Release](https://img.shields.io/github/v/release/fjacquet/pflex_exporter?sort=semver)](https://github.com/fjacquet/pflex_exporter/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A Go exporter for **Dell PowerFlex Gen1** clusters. It authenticates to the PowerFlex
gateway REST API, collects the full Dell statistic set across all object types, and
exposes the metrics via **both** a Prometheus `/metrics` endpoint **and** an OTLP metric
push. It replaces Dell's Python + Telegraf + InfluxDB collection layer, following the
architecture of [`nbu_exporter`](https://github.com/fjacquet/nbu_exporter).

## Features

- **Dual export**: Prometheus pull (`/metrics`) and OTLP metric push, fed from one shared snapshot.
- **Full parity**: all 7 object types — System (cluster), SDS, SDC, Device, Volume, StoragePool, ProtectionDomain.
- **Multi-cluster**: one process monitors many clusters; every metric carries a `cluster` label.
- **Gen1-only**: detects Gen1 (FineGranularity/MediumGranularity) vs Gen2 (ErasureCoding); Gen2 clusters are flagged and skipped without killing the process.
- **Operational**: per-cluster OAuth token lifecycle, graceful per-cluster degradation, hot config reload (SIGHUP + file watch), snapshot-based health endpoint, optional OTLP tracing.

## Architecture

A single background **collection loop** polls every cluster on `collection.interval`,
derives metrics, and publishes an immutable **snapshot** (RWMutex pointer-swap). Both the
Prometheus collector (`Collect()` reads the snapshot) and the OTLP exporter (observable
gauges read the snapshot) serve from that snapshot, so API load is independent of the
number of scrapers and push cadence.

```
PowerFlex gateways ──► ClusterClient (auth + REST) ──► collection loop ──► SnapshotStore
                                                                              │      │
                                                                  Prometheus ◄┘      └► OTLP push
```

## Configuration

See `config.yaml`. Cluster passwords support `${ENV_VAR}` interpolation or a `passwordFile`.

```yaml
server:        { host: "0.0.0.0", port: "2112", uri: "/metrics", logName: "" }
collection:    { interval: "10s", timeout: "8s" }
opentelemetry:
  metrics:     { enabled: false, endpoint: "localhost:4317", insecure: true, interval: "10s" }
  tracing:     { enabled: false, endpoint: "localhost:4317", insecure: true, samplingRate: 0.1 }
clusters:
  - { name: flex-cluster1, gateway: <ip-or-host>, username: <monitor-user>, password: "${FLEX1_PASSWORD}", insecureSkipVerify: true }
```

A `monitor`-type PowerFlex user is sufficient. For PowerFlex 4.0+, `gateway` is the
primary ingress IP (PowerFlex Manager UI).

## Running

```bash
make cli
FLEX1_PASSWORD=... ./bin/pflex_exporter --config config.yaml
# metrics at http://localhost:2112/metrics, health at /health

# one-shot collection (no server):
./bin/pflex_exporter --config config.yaml --once
```

Docker / compose (exporter + Prometheus + OTLP collector):

```bash
FLEX1_PASSWORD=... docker compose up --build
```

## Metric naming

- Names are `pflex_<object>_<metric>`: `pflex_cluster_*`, `pflex_sds_*`, `pflex_sdc_*`,
  `pflex_volume_*`, `pflex_storagepool_*`, `pflex_device_*`, `pflex_protectiondomain_*`.
- Bandwidth/IO accumulators (`*Bwc`) become three gauges with `op` and `direction` labels:
  `_iops`, `_bandwidth_kb_per_second`, `_io_size_kb`. Latency accumulators become `_latency`.
- Other statistics become scalar gauges named from the API field, e.g.
  `pflex_cluster_max_capacity_in_kb`, `pflex_storagepool_spare_capacity_in_kb`.
- Health/meta: `pflex_up{cluster}`, `pflex_last_scrape_timestamp_seconds{cluster}`,
  `pflex_cluster_generation{cluster,generation}`.

**IOPS and bandwidth are already per-second gauges** (derived from the API's counters).
In PromQL, aggregate them with `sum`/`avg` — **never** `rate()`.

## Dashboards

`grafana/` contains importable PromQL dashboards: `pflex-cluster-overview.json` and
`pflex-storage-pools.json`. They demonstrate the metric/label conventions; additional
per-object dashboards (devices, volumes, SDC, SDS, protection domains, capacity planning)
can be built mechanically from the same naming scheme.

## Development

```bash
make tools         # install golangci-lint, cyclonedx-gomod, govulncheck (pinned)
make sure          # fmt + vet + test + build + golangci-lint
make test-coverage # race tests + HTML coverage report
```

## CI/CD & SBOM

Everything CI runs is a Makefile target, so it reproduces locally.

- **CI** (`.github/workflows/ci.yml`, on push/PR) runs `make ci` —
  `gofmt` check, `go vet`, `golangci-lint`, `go test -race` with coverage, and
  `govulncheck` — plus a Semgrep scan and an SBOM artifact.
- **Release** (`.github/workflows/release.yml`, on `v*` tags):
  - `make release` cross-compiles `linux/{amd64,arm64}` and `darwin/{amd64,arm64}`
    binaries, generates the SBOM, and writes `checksums.txt`; all are attached to the
    GitHub Release.
  - A multi-arch container image is pushed to `ghcr.io/<owner>/pflex_exporter` with
    build-time **SBOM and provenance attestations**.
- **SBOM**: `make sbom` produces a CycloneDX SBOM (`dist/sbom.cdx.json`) for the Go
  module via `cyclonedx-gomod`; the container image carries its own SBOM attestation.
- **Version**: injected at build time via `-ldflags "-X main.version=$(VERSION)"`
  (`VERSION` defaults to `git describe`); check with `pflex_exporter --version`.

## Notes

- Supported baseline is **PowerFlex 4.5+** (Gen1). Auth uses the 4.x bearer flow
  (`/rest/auth/login`, `/rest/auth/update-token`); older firmware that only offers Basic
  auth is out of scope.
- The PowerFlex API spells the occurrence counter `numOccured`; the exporter also accepts
  the corrected `numOccurred`.

## License

Apache License 2.0 — see [LICENSE](LICENSE). This matches the license of Dell's upstream
PowerFlex Gen1 monitoring tooling that this exporter is derived from.
