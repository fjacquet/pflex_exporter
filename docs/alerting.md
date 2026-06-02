# Alerting

`pflex_exporter` ships a starter Prometheus alert rule set at
`deploy/prometheus/pflex.rules.yml`, wired into the bundled Compose stack
(`prometheus.yml` references it; the file is mounted into the Prometheus container).

## Shipped alerts

| Alert | Trigger | Severity |
|---|---|---|
| `PflexSdsUnhealthy` | `pflex_sds_health > 0` or `pflex_storagenode_health > 0` | warning |
| `PflexDeviceFailed` | `pflex_device_health >= 2` | critical |
| `PflexSdcDisconnected` | `pflex_sdc_health > 0` | warning |
| `PflexStoragePoolCapacityHighGen1` | Gen1 pool in-use / max `> 0.85` | warning |
| `PflexStoragePoolCapacityCriticalGen1` | Gen1 pool in-use / max `> 0.95` | critical |
| `PflexStoragePoolCapacityHighGen2` | `pflex_storagepool_utilization_ratio > 0.85` | warning |
| `PflexRebuildActive` | rebuild IOPS `> 0` for 10m | info |
| `PflexVolumeReadLatencyHigh` | volume read latency over threshold | warning |

Thresholds are tunable defaults — copy the file and adjust `expr`/`for` to your SLOs.

!!! warning "Do not use `rate()`"
    `pflex_*_iops` and bandwidth metrics are already per-second; aggregate with
    `sum`/`avg`, never `rate()`.

## Using the rules outside Compose

Point any Prometheus at the file via `rule_files:`, or import it into Grafana
(Alerting → Alert rules → import a Prometheus rule group). It depends only on metrics
exposed on `/metrics`.
