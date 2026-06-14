# Dashboards

Importable Grafana dashboards (PromQL) live in `grafana/`, split by generation:

- `grafana/gen1/` (8): cluster overview, devices, storage pools, SDC, SDS, volumes,
  protection domains, cluster capacity.
- `grafana/gen2/` (8): cluster overview, clusters (stacked), devices, pools, SDC/hosts,
  storage node, volumes, cluster capacity.

Use the **gen1** dashboards for mirroring clusters and the **gen2** dashboards for
erasure-coding clusters — the gen2 dashboards use the unit-explicit Gen2 metric names
(bytes/s, bytes, µs). They are JSON-validated; validate against your Grafana before
relying on them.

## Layout & panel types

Every dashboard follows the same rubric, organized into collapsible rows:
**Health → Performance → Capacity/Resilience → Inventory** (each dashboard includes the
rows relevant to its object). Panel-type palette:

- **timeseries** — rates, capacity-over-time, latency, IOPS/bandwidth.
- **stat** — headline KPIs (threshold-colored, with a sparkline).
- **gauge** — bounded percentages only (e.g. Capacity Used %).
- **state-timeline** — `pflex_*_health` over time as green/yellow/red bands
  (0=Healthy, 1=Degraded, 2=Failed), so you can see *when* a component degraded.
- **table** — label-rich `*_info` data (e.g. device state + temperature/SSD-EOL/error).

## Regenerating

The dashboards are generated from a single script so a panel-type change propagates to all
16 at once: `python3 scripts/dashboards/generate.py` (run from the repo root). It rewrites
each dashboard's `panels` while preserving its `uid`, `title`, and template variables, and
fails on any gridPos overlap. Edit the builders/specs in that script rather than hand-editing
the JSON.

## Import

In Grafana: **Dashboards → New → Import**, upload the JSON, and select your Prometheus
data source. Dashboards include a `cluster` template variable (and pool dashboards a
`pool` variable) populated via `label_values(...)`.

These dashboards were audit-validated (0 dead panels — every panel query references a
metric the exporter actually emits). The authoritative concrete metric-name set is
generated from the collector via the dump test
(`PFLEX_DUMP_METRICS=1 go test ./internal/powerflex/ -run TestDumpEmittedMetricNames`),
not from `docs/metrics.md`, which documents metric families rather than every concrete name.

## PromQL conventions

- Metrics are gauges; identity/parent labels let you filter per object.
- `iops` and `bandwidth_kb_per_second` are **pre-derived per-second** values — aggregate
  with `sum`/`avg by (...)`, never `rate()`.

Examples:

```promql
# cluster capacity used %
100 * sum by (cluster) (pflex_cluster_capacity_in_use_in_kb)
    / sum by (cluster) (pflex_cluster_max_capacity_in_kb)

# total read+write IOPS per cluster
sum by (cluster, direction) (pflex_cluster_iops{op="total"})

# busiest volumes by total bandwidth
topk(10, sum by (volume_name) (pflex_volume_bandwidth_kb_per_second{op="userData"}))
```

## Building more

The dashboards follow the [metric naming scheme](metrics.md); new panels can be built
mechanically from it. Remember that Gen2 uses unit-explicit names
(`_bandwidth_bytes_per_second`, `_io_size_bytes`, `_latency_microseconds`) and different
`op` values (`host`, `device`, `storage_fe`, `device_pmem`, `wrc`, …).
