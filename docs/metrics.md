# Metrics Reference

All metrics are gauges named `pflex_<object>_<metric>` and carry a `cluster` label (the
configured cluster name) plus `cluster_id` (the PowerFlex System id).

## Object prefixes

| PowerFlex type | Metric prefix |
|---|---|
| System (cluster) | `pflex_cluster_` |
| SDS | `pflex_sds_` |
| SDC | `pflex_sdc_` |
| Volume | `pflex_volume_` |
| StoragePool | `pflex_storagepool_` |
| Device | `pflex_device_` |
| ProtectionDomain | `pflex_protectiondomain_` |

## Identity & parent labels

Beyond `cluster` / `cluster_id`, each object type adds identity and parent labels,
resolved from the PowerFlex relationship graph:

| Type | Additional labels |
|---|---|
| SDS | `sds`, `sds_id`, `protection_domain_name`, `protection_domain_id` |
| SDC | `sdc_name`, `sdc_id` (name falls back to the SDC IP, then id) |
| Volume | `volume_name`, `volume_id`, `storage_pool_name`, `storage_pool_id`, `protection_domain_name`, `protection_domain_id` |
| StoragePool | `storage_pool_name`, `storage_pool_id`, `protection_domain_name`, `protection_domain_id` |
| Device | `device_name`, `device_id`, `device_path`, `sds`, `sds_id`, `storage_pool_name`, `storage_pool_id`, `protection_domain_name`, `protection_domain_id` |
| ProtectionDomain | `protection_domain_name`, `protection_domain_id` |

## Derived performance metrics

PowerFlex reports `*Bwc` accumulators (`numOccured`, `numSeconds`, `totalWeightInKb`).
Each becomes three gauges, plus `*Latency` accumulators become one:

| Metric | Derivation | Extra labels |
|---|---|---|
| `pflex_<obj>_iops` | `numOccured / numSeconds` (per second) | `op`, `direction` |
| `pflex_<obj>_bandwidth_kb_per_second` | `totalWeightInKb / numSeconds` (KB/s) | `op`, `direction` |
| `pflex_<obj>_io_size_kb` | `totalWeightInKb / numOccured` (avg KB) | `op`, `direction` |
| `pflex_<obj>_latency` | `totalWeightInKb / numOccured` | `op`, `direction` |

- `direction` is `read` or `write` (or empty for accumulators with neither).
- `op` is the accumulator prefix: `primary`, `total`, `fwdRebuild`, `bckRebuild`,
  `normRebuild`, `rebalance`, `volMigration`, `userData`, `userDataSdc`, …

!!! warning "Do not use `rate()`"
    `iops` and `bandwidth_kb_per_second` are **already per-second** values computed by
    PowerFlex. In PromQL aggregate them with `sum` / `avg` by labels — never wrap them in
    `rate()` or you will double-rate.

## Scalar metrics

Every non-accumulator statistic becomes a scalar gauge named from the API field in
snake_case, e.g.:

- `pflex_cluster_max_capacity_in_kb`, `pflex_cluster_capacity_in_use_in_kb`,
  `pflex_cluster_unused_capacity_in_kb`, `pflex_cluster_spare_capacity_in_kb`,
  `pflex_cluster_compression_ratio`, …
- `pflex_storagepool_max_capacity_in_kb`, `pflex_storagepool_spare_capacity_in_kb`, …
- `pflex_sdc_num_of_mapped_volumes`
- `pflex_device_avg_read_size_in_bytes`, `pflex_device_avg_read_latency_in_microsec`, …

Capacity values are reported in **KiB** (`*_in_kb`), matching the PowerFlex API. Multiply
by 1024 in PromQL if you need bytes.

## Health & meta metrics

| Metric | Labels | Meaning |
|---|---|---|
| `pflex_up` | `cluster` | `1` if the cluster was scraped successfully this cycle, else `0`. |
| `pflex_last_scrape_timestamp_seconds` | `cluster` | Unix time of the last successful collection. |
| `pflex_cluster_generation` | `cluster`, `generation` | Always `1`; the `generation` label is `gen1`, `gen2`, or `unknown`. |

A `gen2` cluster reports `pflex_up=1` and `pflex_cluster_generation{generation="gen2"}=1`
but produces no statistic metrics (it is out of scope for this Gen1 exporter).
