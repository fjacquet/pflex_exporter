# Dashboards

Importable Grafana dashboards (PromQL) live in `grafana/`:

| File | Dashboard |
|---|---|
| `pflex-cluster-overview.json` | Cluster capacity, IOPS, bandwidth, latency, rebuild/rebalance, compression |
| `pflex-storage-pools.json` | Per-pool capacity, spare, IOPS and bandwidth |

## Import

In Grafana: **Dashboards → New → Import**, upload the JSON, and select your Prometheus
data source. Both dashboards include a `cluster` template variable (and the storage-pool
dashboard a `pool` variable) populated via `label_values(...)`.

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

The two dashboards demonstrate the metric/label conventions; additional per-object
dashboards (devices, volumes, SDC, SDS, protection domains, capacity planning) can be
built mechanically from the same [metric naming scheme](metrics.md).
