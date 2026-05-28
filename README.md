# pflex_exporter

[![CI](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/ci.yml)
[![Release](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml/badge.svg)](https://github.com/fjacquet/pflex_exporter/actions/workflows/release.yml)
[![Docs](https://github.com/fjacquet/pflex_exporter/actions/workflows/docs.yml/badge.svg)](https://fjacquet.github.io/pflex_exporter/)
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
- **Gen1-only**: detects Gen1 vs Gen2 (ErasureCoding); Gen2 clusters are flagged and skipped without killing the process.
- **Operational**: per-cluster OAuth token lifecycle, graceful degradation, hot config reload (SIGHUP + file watch), snapshot-based health endpoint, optional OTLP tracing.

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

- Supported baseline is **PowerFlex 4.5+** (Gen1), using the 4.x bearer auth flow.
- IOPS and bandwidth are already per-second gauges — aggregate with `sum`/`avg` in PromQL, never `rate()`.

## License

Apache License 2.0 — see [LICENSE](LICENSE). Matches Dell's upstream PowerFlex Gen1
monitoring tooling that this exporter is derived from.
