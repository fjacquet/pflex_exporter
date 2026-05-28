# PowerFlex Exporter

A Go exporter for **Dell PowerFlex Gen1** clusters. It authenticates to the PowerFlex
gateway REST API, collects the full Dell statistic set across all object types, and
exposes the metrics via **both** a Prometheus `/metrics` endpoint **and** an OTLP metric
push. It replaces Dell's Python + Telegraf + InfluxDB collection layer.

## Features

- **Dual export** — Prometheus pull (`/metrics`) and OTLP metric push, fed from one shared snapshot.
- **Full parity** — all 7 object types: System (cluster), SDS, SDC, Device, Volume, StoragePool, ProtectionDomain.
- **Multi-cluster** — one process monitors many clusters; every metric carries a `cluster` label.
- **Gen1-only** — detects Gen1 (FineGranularity/MediumGranularity) vs Gen2 (ErasureCoding); Gen2 clusters are flagged and skipped without killing the process.
- **Operational** — per-cluster OAuth token lifecycle, graceful per-cluster degradation, hot config reload (SIGHUP + file watch), snapshot-based health endpoint, optional OTLP tracing.

## Architecture

A single background **collection loop** polls every cluster on `collection.interval`,
derives metrics, and publishes an immutable **snapshot** (RWMutex pointer-swap). Both the
Prometheus collector (`Collect()` reads the snapshot) and the OTLP exporter (observable
gauges read the snapshot) serve from that snapshot, so API load is independent of the
number of scrapers and the push cadence.

```
PowerFlex gateways ──► ClusterClient (auth + REST) ──► collection loop ──► SnapshotStore
                                                                              │      │
                                                                  Prometheus ◄┘      └► OTLP push
```

The cost of the snapshot model is up to one `collection.interval` of staleness on a
scrape (acceptable — Dell's own pipeline polls every 10s); it is surfaced via
`pflex_last_scrape_timestamp_seconds`.

## Scope

Supported baseline is **PowerFlex 4.5+** (Gen1). Authentication uses the 4.x bearer flow
(`/rest/auth/login`, `/rest/auth/update-token`); older firmware offering only Basic auth
is out of scope.

See [Installation](getting-started/installation.md) to get started.
