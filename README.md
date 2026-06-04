# pflex_exporter

[![CI](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml)
[![Release](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml)
[![Docs](https://github.com/fjacquet/pflex_exporter/actions/workflows/docs.yml/badge.svg)](https://fjacquet.github.io/pflex_exporter/)
[![Go Report Card](https://goreportcard.com/badge/github.com/fjacquet/pflex_exporter)](https://goreportcard.com/report/github.com/fjacquet/pflex_exporter)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fjacquet/pflex_exporter)](go.mod)
[![Latest Release](https://img.shields.io/github/v/release/fjacquet/pflex_exporter?sort=semver)](https://github.com/fjacquet/pflex_exporter/releases)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

A Go exporter for **Dell PowerFlex** clusters (**Gen1 and Gen2**). It authenticates to the
PowerFlex gateway REST API, collects the full Dell statistic set across all object types,
and exposes the metrics via **both** a Prometheus `/metrics` endpoint **and** an OTLP
metric push. It replaces Dell's Python + Telegraf + InfluxDB collection layer, following
the architecture of [`nbu_exporter`](https://github.com/fjacquet/nbu_exporter).

## Features

- **Dual export**: Prometheus pull (`/metrics`) and OTLP metric push, fed from one shared snapshot.
- **Both generations, auto-detected**: Gen1 (mirroring) via `querySelectedStatistics`, Gen2 (erasure coding) via the v5 metrics API — chosen per cluster from storage-pool layout.
- **Full parity**: Gen1's 7 object types plus Gen2's StorageNode, DeviceGroup (PMEM/WRC) and Sdt (NVMe/TCP).
- **Multi-cluster**: one process monitors many clusters (mixed generations OK); every metric carries a `cluster` label.
- **Kubernetes-aware (optional)**: enrich volume metrics with `namespace` / `persistent_volume_claim` / `persistent_volume` / `storage_class` and SDC metrics with `k8s_node`, mapped from the cluster's PVs/Nodes via the PowerFlex CSI driver. Portable — a no-op when run standalone.
- **Scales to large estates**: configurable cap on concurrent cluster polling, plus optional decimation (collect slow-changing Gen2 resource types every Nth cycle) to bound API load.
- **Operational**: per-cluster OAuth token lifecycle, graceful degradation, stats-path fallback (Gen1↔Gen2), hot config reload (SIGHUP + file watch), snapshot-based health endpoint, optional OTLP tracing.

## Quick start

```bash
make cli
export FLEX1_PASSWORD='your-monitor-password'
./bin/pflex_exporter --config config.yaml
# metrics: http://localhost:2112/metrics   health: http://localhost:2112/health
```

Or with Docker: `docker pull ghcr.io/fjacquet/pflex_exporter:latest`.

## Documentation

Full docs at **<https://fjacquet.github.io/pflex_exporter/>**:

- [Installation](https://fjacquet.github.io/pflex_exporter/getting-started/installation/) ·
  [Configuration](https://fjacquet.github.io/pflex_exporter/getting-started/configuration/) ·
  [Quick Start](https://fjacquet.github.io/pflex_exporter/getting-started/quickstart/)
- [Metrics Reference](https://fjacquet.github.io/pflex_exporter/metrics/)
- Deployment:
  [Docker](https://fjacquet.github.io/pflex_exporter/deployment/docker/) ·
  [systemd](https://fjacquet.github.io/pflex_exporter/deployment/systemd/) ·
  [Kubernetes](https://fjacquet.github.io/pflex_exporter/deployment/kubernetes/)
- [Dashboards](https://fjacquet.github.io/pflex_exporter/dashboards/) ·
  [OpenTelemetry](https://fjacquet.github.io/pflex_exporter/opentelemetry/) ·
  [CI/CD & SBOM](https://fjacquet.github.io/pflex_exporter/cicd/)

Deployment manifests and examples live in [`deploy/`](deploy/); Grafana dashboards in
[`grafana/`](grafana/).

## Development

```bash
make tools         # install golangci-lint, cyclonedx-gomod, govulncheck (pinned)
make sure          # fmt + vet + test + build + golangci-lint
make ci            # the gate CI runs (adds go test -race + govulncheck)
```

## Notes

- Supports **PowerFlex 4.5+ (Gen1)** and **5.x+ (Gen2)**, both via the bearer auth flow; generation is auto-detected per cluster.
- IOPS and bandwidth are already per-second gauges — aggregate with `sum`/`avg` in PromQL, never `rate()`.
- Gen2 uses unit-explicit metric names (`_bandwidth_bytes_per_second`, `_io_size_bytes`, `_latency_microseconds`); Gen1 uses KB-based names.

## License

Apache License 2.0 — see [LICENSE](LICENSE). Matches Dell's upstream PowerFlex
monitoring tooling that this exporter is derived from.
