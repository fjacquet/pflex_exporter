# Observability Completeness — State Gauges, Volume↔SDC Mapping, Alert Rules

**Date:** 2026-06-02
**Status:** Approved (design)
**Origin:** Comparison against Dell `karavi-metrics-powerflex`, scoped to mission-aligned, k8s-free improvements.

## Background

Dell's `karavi-metrics-powerflex` is a Kubernetes/CSI-driver observability tool: its value is correlating PowerFlex metrics with k8s objects (PVs, nodes, storage classes) and exposing capacity/topology/availability signals. `pflex_exporter` is a general-purpose **array** exporter with dual export (native Prometheus `/metrics` + OTLP push), Gen1/Gen2 auto-detection, and no k8s coupling.

A comparison surfaced three real, mission-aligned gaps. Capacity is **already covered** by the exporter, so it is out of scope. The gaps:

- **A — Object health/state gauges.** The exporter is rich on performance and capacity counters but exposes almost no *operational state* as numeric gauges. You cannot currently alert on "an SDS went down" — only on performance changing.
- **B — Volume↔SDC mapping.** No metric links a volume to the SDC(s)/hosts consuming it. This is Dell's signature capability, achievable here without k8s.
- **C — Curated alert rules.** 16 Grafana dashboards ship, but no alert rules. State gauges (A) make the most valuable alerts possible.

Explicitly **not** adopted: read/write split metric names (our `op`/`direction` labels are the Prometheus-preferred form), k8s leader election (does not fit the snapshot model), gomock refactor (the httptest mock gateway is equivalent).

## Goals

1. Expose object operational state as numeric **health gauges** plus companion **info metrics** carrying raw state strings.
2. Expose **volume→SDC mapping** as an info metric for host-level IO correlation, without k8s.
3. Ship a **starter set of Prometheus alert rules** built on the new health/capacity signals.
4. Keep docs, dashboards, and `CLAUDE.md` in lockstep with the new metrics.

## Non-goals

- No k8s/CSI integration (no PV/StorageClass/Node correlation).
- No new external API calls — all new data comes from the **already-fetched** `GET /api/instances` payload.
- No change to the snapshot model, auth flow, or the two export paths.

## Architecture

### Second derivation path: instance properties → samples

Today every `Sample` originates from a **statistics** query (`deriveSamples` for Gen1, `deriveSamplesV5` for Gen2). This design adds a parallel path that derives samples from **instance properties** already present in the `GET /api/instances` response:

```
GET /api/instances ──┬─► (existing) querySelectedStatistics / dtapi metrics ─► deriveSamples* ─► []Sample ─┐
                     └─► (NEW) instance properties ──────────────────────────► deriveStateSamples ─► []Sample ─┼─► snapshot
```

- **New file:** `internal/powerflex/state.go`.
- **Entry point:** `deriveStateSamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, gen Generation) []Sample`.
- **Wiring:** `collectCluster` (both Gen1 and Gen2 branches in `collector.go`) appends `deriveStateSamples(...)` output to the existing sample slice before publishing the snapshot. Snapshot store, `PromCollector`, and `OTLPExporter` are untouched — they already consume `[]Sample`.
- **Label reuse:** state/info/mapping series are labelled by the **existing `labelBuilder` functions** in `metrics.go`. This guarantees identity/parent labels match the performance metrics and that Gen1/Gen2 share the same canonical union label-key sets (load-bearing for mixed-generation `/metrics`; see `deviceLabels`/`volumeLabels`).

### Feature A — health gauge + info metric

For each in-scope object, emit two series:

- `pflex_<obj>_health` — numeric severity: **0 = healthy, 1 = degraded, 2 = failed/disconnected/unknown**. A missing or unrecognized state maps to `2` (unknown), so a missing signal is visible and alertable, never silently healthy.
- `pflex_<obj>_info{<state-string labels...>} 1` — raw PowerFlex state strings preserved as label values (e.g. `mdm_state="Connected"`, `membership="Joined"`, `maintenance="NoMaintenance"`). Value is always `1` (standard info-metric convention).

**Severity mapping** lives as a data-driven table in `state.go`: `map[stateString]severity`. The health gauge value is the **max severity** across the object's relevant state fields (worst-state-wins).

**In-scope objects and state fields (first slice):**

| Object | Health metric | State fields (info) |
|---|---|---|
| SDS (Gen1) | `pflex_sds_health` | `mdmConnectionState`, `membershipState`, `maintenanceState` |
| StorageNode (Gen2) | `pflex_storagenode_health` | `mdmConnectionState`, `membershipState`, `maintenanceState` |
| Device (both) | `pflex_device_health` | `deviceState` |
| SDC (both) | `pflex_sdc_health` | `mdmConnectionState` |

Extensible to `Sdt` and others later by adding a row to the table and the field to the struct. `Device` and `SDC` health/info metrics are produced by both generations, so they MUST use the shared union label builders (`deviceLabels`, `buildSdcLabels`) — same constraint that governs the existing performance metrics.

Exact JSON field names are verified against the PowerFlex 4.5+ (Gen1) and 5.x (Gen2) REST API during planning, before implementation.

### Feature B — volume↔SDC mapping

`Volume` instances carry `mappedSdcInfo`, an array of `{sdcId, sdcIp}` entries. Emit one info series per mapping:

```
pflex_volume_mapped_sdc{volume_name, volume_id, volume_type, storage_pool_id, storage_pool_name, protection_domain_id, protection_domain_name, sdc_id, sdc_ip} 1
```

Volume identity/parent labels come from the existing union `volumeLabels` set (Gen1 passes empty `volume_type`), so the metric is label-consistent across generations. The `sdc_id`/`sdc_ip` labels are appended consistently in both generations. An unmapped volume emits no mapping series.

This enables PromQL joins from volume IO to consuming host without k8s, e.g. correlating `pflex_volume_*_iops` with the SDC driving it.

### Feature C — starter alert rules

Ship a portable **Prometheus rule file**: `deploy/prometheus/pflex.rules.yml`, wired into the compose Prometheus config. Prometheus-native rules are portable to any Prometheus/Alertmanager and can also be imported into Grafana; this is preferred over Grafana-only provisioning (the empty `grafana/provisioning/alerting/` dir).

Starter rules:

| Alert | Expression (illustrative) | Severity |
|---|---|---|
| SDS / StorageNode unhealthy | `pflex_sds_health > 0` / `pflex_storagenode_health > 0` | warning (1) / critical (2) by value |
| Device failed | `pflex_device_health >= 2` | critical |
| SDC disconnected | `pflex_sdc_health > 0` | warning |
| Capacity pressure | `in_use / limit > 0.85` / `> 0.95` | warning / critical |
| Rebuild/rebalance active | rebuild-pending capacity `> 0` | info |
| Latency SLO breach | read/write latency over threshold | warning |

Exact metric names for capacity/rebuild/latency expressions are confirmed against the current exporter output during planning. Thresholds are documented as tunable defaults.

### Instance struct extension

`models.Instance` gains the state fields and a mapping slice:

```go
// SDS / StorageNode operational state
MdmConnectionState string `json:"mdmConnectionState,omitempty"`
MembershipState    string `json:"membershipState,omitempty"`
MaintenanceState   string `json:"maintenanceState,omitempty"`
// Device
DeviceState        string `json:"deviceState,omitempty"`
// Volume → SDC mapping
MappedSdcInfo      []MappedSdc `json:"mappedSdcInfo,omitempty"`
```

with `type MappedSdc struct { SdcID string `json:"sdcId"`; SdcIP string `json:"sdcIp"` }`. Fields are additive; `ParseInstances` continues to ignore unmodeled JSON. SDC reuses `MdmConnectionState`.

## Error handling & degradation

- **Missing/empty state field →** treated as `unknown` (severity 2), surfaced rather than hidden.
- **Unresolvable parent chain →** object skipped, identical to existing behavior (`labelBuilder` returns `ok=false`).
- **Unmapped volume →** no mapping series emitted (absence, not a zero series).
- New path never fails collection: a panic-free pure transform over already-parsed data.

## Testing

- Extend `testdata/instances*.json` (Gen1 and Gen2) with the new state fields and `mappedSdcInfo` arrays.
- New tests assert: health gauge numeric values (healthy/degraded/failed/unknown), info-metric label values, and mapping series — verified via **both** the Prometheus registry gather **and** the OTLP `ManualReader`, matching existing collector-test conventions.
- Extend `TestMixedGenerationMetricsValid` to cover the new dual-generation metrics (`pflex_device_health`, `pflex_sdc_health`, `pflex_volume_mapped_sdc`) for label-key consistency.
- Test HTTP handlers continue to write fixtures via the `writeBytes(io.Writer, …)` helper (semgrep rule).

## Documentation (kept in lockstep)

- `docs/metrics.md` — document `pflex_<obj>_health`, `pflex_<obj>_info`, and `pflex_volume_mapped_sdc`, including the health severity encoding.
- New alerting section/page — describe the shipped rule file, the alerts, and tunable thresholds.
- `CLAUDE.md` — extend "Adding metrics or object types" with the new instance-property derivation path (`state.go`) and the severity table.
- Grafana dashboards — add an optional health/status panel.

## Build order (single spec, three stages)

1. **A** — Instance struct extension + `state.go` (`deriveStateSamples`, severity table) + health/info gauges + collector wiring + tests + docs.
2. **B** — `mappedSdcInfo` parsing + `pflex_volume_mapped_sdc` emission (reuses A's path) + tests + docs.
3. **C** — `deploy/prometheus/pflex.rules.yml` + compose wiring + alerting docs + optional dashboard panel.

## Constraints carried from CLAUDE.md

- `iops`/bandwidth are per-second gauges — alert expressions use `sum`/`avg`, never `rate()`.
- Prometheus label-key consistency: any metric name emitted by both generations carries one union label-key set in fixed canonical order.
- resty retry must continue to exclude 4xx; no new auth retries.
- Semgrep runs on every file write and blocks on findings; inline suppression is not honored — fix by restructuring.
- Dockerfiles must declare a non-root `USER` (no Dockerfile change expected here).
