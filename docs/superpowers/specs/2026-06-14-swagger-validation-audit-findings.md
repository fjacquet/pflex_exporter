# Swagger-validation audit findings — 2026-06-14

## WS3 — Grafana dashboard validation

Cross-check of every `pflex_*` metric referenced by the 16 Grafana dashboards
(`grafana/gen1/*.json`, `grafana/gen2/*.json`) against the metrics the exporter
emits. The emitted set is a fixture-derived **lower bound** (from
`TestDumpEmittedMetricNames`: 45 gen1 + 36 gen2 concrete names), so every
dead candidate was re-verified against source before classification.

### Dead panels

| ID | Metric | Dashboard file(s) | Classification | Correct name (if rename) | Tier |
|----|--------|-------------------|----------------|--------------------------|------|
| WS3-01 | `pflex_exporter` | (all dashboards — dashboard `tags` array, not a query) | FALSE POSITIVE | n/a — it is the dashboard tag `"pflex_exporter"`, never a metric reference | — |
| WS3-02 | `pflex_cluster_data_reduction_ratio` | gen2/08-cluster-capacity.json, gen2/01-cluster-overview.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["data_reduction_ratio"]` (statistics_v5.go:146), scalar → `pflex_cluster_data_reduction_ratio` | — |
| WS3-03 | `pflex_cluster_efficiency_ratio` | gen2/08-cluster-capacity.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["efficiency_ratio"]` (statistics_v5.go:147) | — |
| WS3-04 | `pflex_cluster_logical_provisioned` | gen2/08-cluster-capacity.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["logical_provisioned"]` (statistics_v5.go:141) | — |
| WS3-05 | `pflex_cluster_logical_used` | gen2/08-cluster-capacity.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["logical_used"]` (statistics_v5.go:140) | — |
| WS3-06 | `pflex_cluster_physical_free` | gen2/08-cluster-capacity.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["physical_free"]` (statistics_v5.go:138) | — |
| WS3-07 | `pflex_cluster_physical_total` | gen2/08-cluster-capacity.json, gen2/01-cluster-overview.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSystem]["physical_total"]` (statistics_v5.go:136) | — |
| WS3-08 | `pflex_cluster_spare_capacity_in_kb` | gen1/08-cluster-capacity.json | FIXTURE-ONLY | n/a — gen1 `System` stat `spareCapacityInKb` (querySelectedStatistics.json); `toSnake` → `spare_capacity_in_kb` | — |
| WS3-09 | `pflex_cluster_thick_capacity_in_use_in_kb` | gen1/08-cluster-capacity.json | FIXTURE-ONLY | n/a — gen1 `System` stat `thickCapacityInUseInKb` (querySelectedStatistics.json) | — |
| WS3-10 | `pflex_cluster_thin_capacity_allocated_in_kb` | gen1/08-cluster-capacity.json | FIXTURE-ONLY | n/a — gen1 `System` stat `thinCapacityAllocatedInKb` (querySelectedStatistics.json) | — |
| WS3-11 | `pflex_device_bandwidth_bytes_per_second` | gen2/03-devices.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeDevice]` `*_bandwidth` (v5KindBW) → `_bandwidth_bytes_per_second` (derivations_v5.go:18) | — |
| WS3-12 | `pflex_device_io_size_bytes` | gen2/03-devices.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeDevice]` `avg_device_*_io_size` (v5KindIOSize) → `_io_size_bytes` (derivations_v5.go:20) | — |
| WS3-13 | `pflex_sdc_bandwidth_bytes_per_second` | gen2/05-sdc-hosts.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeSdc]` `host_*_bandwidth` (v5KindBW) | — |
| WS3-14 | `pflex_storagenode_latency_microseconds` | gen2/06-storage-node.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeStorageNode]` `*_latency` (v5KindLatency) → `_latency_microseconds` (derivations_v5.go:22) | — |
| WS3-15 | `pflex_storagepool_bandwidth_bytes_per_second` | gen2/04-pools.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeStoragePool]` `*_bandwidth` (v5KindBW) | — |
| WS3-16 | `pflex_storagepool_data_reduction_ratio` | gen2/04-pools.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeStoragePool]["data_reduction_ratio"]` (statistics_v5.go:242) | — |
| WS3-17 | `pflex_storagepool_physical_total` | gen2/04-pools.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeStoragePool]["physical_total"]` (statistics_v5.go:233) | — |
| WS3-18 | `pflex_volume_logical_provisioned` | gen2/07-volumes.json | FIXTURE-ONLY | n/a — `v5Metrics[TypeVolume]["logical_provisioned"]` (statistics_v5.go:207) | — |

**No TRUE DEAD PANELS and no COVERAGE GAPS found.** Every referenced metric is
either emitted (and seen in the fixture run) or provably emittable from source
(the gen2 capacity/efficiency scalars and unit-suffixed gen2 bandwidth/io-size/
latency metrics are real; the gen1 fixture and the gen2 fixture each only
populate a subset of object types/stats). `pflex_exporter` (WS3-01) is the
dashboard's `tags` value, matched spuriously by the extraction regex — not a
query reference.

### Uncovered metrics (emitted, no panel — reported only)

These metrics are emitted by the exporter but referenced by no dashboard panel:

- `pflex_cluster_io_size_kb`
- `pflex_cluster_unused_capacity_in_kb`
- `pflex_device_health`
- `pflex_device_info`
- `pflex_device_io_size_kb`
- `pflex_device_raw_total`
- `pflex_devicegroup_iops`
- `pflex_devicegroup_raw_total`
- `pflex_last_scrape_timestamp_seconds`
- `pflex_protectiondomain_bandwidth_bytes_per_second`
- `pflex_protectiondomain_bandwidth_kb_per_second`
- `pflex_protectiondomain_io_size_kb`
- `pflex_protectiondomain_physical_used`
- `pflex_sdc_health`
- `pflex_sdc_info`
- `pflex_sdc_io_size_kb`
- `pflex_sds_health`
- `pflex_sds_info`
- `pflex_sds_io_size_kb`
- `pflex_sdt_latency_microseconds`
- `pflex_storagenode_health`
- `pflex_storagenode_info`
- `pflex_storagenode_raw_total`
- `pflex_storagepool_compression_ratio`
- `pflex_storagepool_io_size_kb`
- `pflex_volume_io_size_kb`
- `pflex_volume_mapped_sdc`

Note: `pflex_devicegroup` and `pflex_sdt` (gen2-only object types) have **no
dashboard at all**, which explains several of the uncovered entries. Health/info
state metrics and `pflex_last_scrape_timestamp_seconds` are operational signals
not surfaced as panels.

### Notes

- Methodology: emitted set is a fixture-derived lower bound; all 18 suspects
  verified against source (`metrics.go` `metricPrefix`/`toSnake`,
  `statistics_v5.go` `v5Metrics` table, `derivations_v5.go` unit-suffix logic,
  `querySelectedStatistics.json` gen1 stats, `derivations.go`).
- Generation-suffix awareness applied: gen1 dashboards reference
  `_kb_per_second`/`_latency`/`_io_size_kb`/`_*_in_kb`; gen2 dashboards reference
  `_bytes_per_second`/`_latency_microseconds`/`_io_size_bytes`. Each dead
  candidate was checked against the **matching** generation's source path.
- Counts: **56 dashboard metrics referenced** (57 extracted names minus the
  `pflex_exporter` tag false positive); **39 confirmed emitted in the fixture
  run**; **0 true dead panels**; **17 fixture-only** (real, emittable from
  source but not populated by the test fixtures); **0 coverage gaps**;
  **27 uncovered** (emitted, no panel). 1 extraction false positive
  (`pflex_exporter`).
