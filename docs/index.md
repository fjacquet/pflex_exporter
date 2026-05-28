# PowerFlex Exporter

A Go exporter for **Dell PowerFlex** clusters (**Gen1 and Gen2**). It authenticates to the
PowerFlex gateway REST API, collects the full Dell statistic set across all object types,
and exposes the metrics via **both** a Prometheus `/metrics` endpoint **and** an OTLP
metric push. It replaces Dell's Python + Telegraf + InfluxDB collection layer.

## Features

- **Dual export** — Prometheus pull (`/metrics`) and OTLP metric push, fed from one shared snapshot.
- **Both generations, auto-detected** — Gen1 (mirroring) via `querySelectedStatistics`, Gen2 (ErasureCoding) via the v5 metrics API; the generation is detected per cluster from storage-pool layout.
- **Full parity** — Gen1's 7 object types plus Gen2's StorageNode, DeviceGroup (incl. PMEM/WRC) and Sdt (NVMe/TCP).
- **Multi-cluster** — one process monitors many clusters; every metric carries a `cluster` label.
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

Supports **PowerFlex 4.5+ (Gen1, mirroring)** and **PowerFlex 5.x+ (Gen2, erasure coding)**.
Both use the bearer auth flow (`/rest/auth/login`, `/rest/auth/update-token`); older
firmware offering only Basic auth is out of scope. The exporter detects each cluster's
generation automatically and chooses the matching collection path — a single binary
covers a fleet of either or both generations.

See [Installation](getting-started/installation.md) to get started.
