# Metrics Reference

All metrics are gauges named `pflex_<object>_<metric>` and carry a `cluster` label (the
configured cluster name) plus `cluster_id` (the PowerFlex System id).

> This page documents metric **families** and conventions. The authoritative, concrete
> emitted metric-name set is generated from the collector via the dump test
> (`PFLEX_DUMP_METRICS=1 go test ./internal/powerflex/ -run TestDumpEmittedMetricNames`),
> not hand-enumerated here.

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

### Kubernetes enrichment labels (optional)

When [Kubernetes workload enrichment](getting-started/configuration.md#kubernetes-workload-enrichment)
is enabled, two object types gain extra labels mapped from the cluster's PVs and Nodes:

| Type | Added labels |
|---|---|
| Volume | `namespace`, `persistent_volume_claim`, `persistent_volume`, `storage_class` |
| SDC | `k8s_node` |

These keys are present on every `pflex_volume_*` / `pflex_sdc_*` series while enrichment is
on (empty when a workload can't be resolved), so the label-key set stays consistent. They
let you aggregate storage performance by namespace, claim, or node — e.g.
`sum by (namespace) (pflex_volume_iops)`.

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

**Redundancy & rebuild/rebalance (Gen1; System / StoragePool / ProtectionDomain):**

- Capacity-at-risk: `pflex_<obj>_degraded_failed_capacity_in_kb`,
  `pflex_<obj>_failed_capacity_in_kb`, `pflex_<obj>_degraded_healthy_capacity_in_kb`,
  `pflex_<obj>_spare_capacity_in_kb`.
- Rebuild/rebalance remaining capacity: `pflex_<obj>_fwd_rebuild_capacity_in_kb`,
  `pflex_<obj>_bck_rebuild_capacity_in_kb`, `pflex_<obj>_norm_rebuild_capacity_in_kb`,
  `pflex_<obj>_rebalance_capacity_in_kb` (the *rate* of rebuild/rebalance remains the
  `*RebuildBwc`-derived `iops`/`bandwidth`).
- Data-movement job counts: `pflex_<obj>_pending_moving_in_bck_rebuild_jobs`,
  `pflex_<obj>_active_moving_in_bck_rebuild_jobs`, and the `out`/`fwd_rebuild`/`rebalance`
  variants.
- Snapshot capacity: `pflex_<obj>_snapshot_capacity_in_kb`,
  `pflex_<obj>_net_snapshot_capacity_in_kb`, `pflex_<obj>_snap_capacity_in_use_in_kb`.

**Back-end latency (Gen1):** `pflex_protectiondomain_latency{op="target"|"journaler"}`
(and `op="target"` on StoragePool) split read/write — complements the host-side
`op="userData"` latency for localizing a latency problem to the device/journal tier.

**Inventory counts (Gen1; from System instance properties):**
`pflex_cluster_num_of_volumes`, `pflex_cluster_num_of_sds`, `pflex_cluster_num_of_devices`.

## Gen2 differences

The generation is detected per cluster (storage-pool `dataLayout`: ErasureCoding = Gen2)
and exposed as `pflex_cluster_generation{generation="gen2"}`. Gen2 collects from the v5
metrics API, where values are **pre-computed**, so the shapes are the same but with
explicit units and Gen2-specific labels:

- **Object prefixes:** Gen1 `pflex_sds_*` becomes `pflex_storagenode_*` (SDS was renamed
  StorageNode). Gen2 adds `pflex_devicegroup_*` (with a `media_type` label, incl.
  PMEM/WRC `op` values) and `pflex_sdt_*` (NVMe/TCP path latency). System, Volume,
  StoragePool, Device, Sdc and ProtectionDomain keep their prefixes.
- **Unit-explicit derived names** (Gen2 values are bytes/s, bytes, µs):
  `pflex_<obj>_iops` (shared), `pflex_<obj>_bandwidth_bytes_per_second`,
  `pflex_<obj>_io_size_bytes`, `pflex_<obj>_latency_microseconds`.
- **`op` values:** `host`, `device`, `device_local`, `device_remote`, `storage_fe`,
  `device_pmem`, `wrc`, `rebuild`, `rebalance`, `controller_to_host`, `host_to_controller`.
  `direction` is `read`/`write`/`trim`/`""`.
- **Scalars** are bytes/ratios named from the v5 field: `pflex_<obj>_physical_used`,
  `_raw_total`, `_logical_provisioned`, `_compression_ratio`, `_data_reduction_ratio`,
  `_utilization_ratio`, etc. (distinct from Gen1's `*_in_kb`).
- **Volume** carries `volume_type` (BaseVolume/ThinClone) and **Device** carries
  `storage_node_name/id`. Both Gen1 and Gen2 forms of these two metrics share a union
  label-key set (inapplicable keys empty) so a single exporter can serve mixed-generation
  fleets and keep `/metrics` valid.

## Health & meta metrics

| Metric | Labels | Meaning |
|---|---|---|
| `pflex_up` | `cluster` | `1` if the cluster was scraped successfully this cycle, else `0`. |
| `pflex_last_scrape_timestamp_seconds` | `cluster` | Unix time of the last successful collection. |
| `pflex_cluster_generation` | `cluster`, `generation` | Always `1`; the `generation` label is `gen1`, `gen2`, or `unknown`. |
| `pflex_<obj>_health` | object identity/parent labels | Operational severity: `0`=healthy, `1`=degraded, `2`=failed/disconnected/unknown. Emitted for SDS/StorageNode, Device, SDC, and **Sdt (Gen2 only)**. An unreported (empty) optional state field carries no signal and does not force `2`. |
| `pflex_<obj>_info` | identity labels + raw state strings | Always `1`; carries raw PowerFlex state strings (`mdm_connection_state`, `membership_state`, `maintenance_state`, `device_state`; Device also `temperature_state`, `ssd_end_of_life_state`, `error_state`; Sdt `sdt_state`). |
| `pflex_volume_mapped_sdc` | volume identity/parent labels + `sdc_id`, `sdc_ip` | Always `1`; one series per volume→SDC mapping, correlating a volume with each host consuming it. |

### Operational state

State gauges are derived from object properties in `GET /api/instances` (not from the
statistics API). The `*_health` value is the **worst severity** across the object's state
fields; the matching `*_info` metric preserves the raw strings as labels. A missing or
unrecognized state maps to `2` (unknown), so a lost signal is surfaced rather than hidden.

Expected state fields are present on PowerFlex 4.5+ (Gen1) and 5.x (Gen2). Alert on
`pflex_<obj>_health > 0`; join to a host with `pflex_volume_mapped_sdc` (e.g.
`pflex_volume_mapped_sdc * on(volume_id) group_left() pflex_volume_iops`, which yields one
result per volume↔SDC pair carrying the volume's IOPS).

An `unknown`-generation cluster (no recognizable storage-pool layout) reports
`pflex_up=1` and `pflex_cluster_generation{generation="unknown"}=1` and falls back to the
Gen1 collection path.
