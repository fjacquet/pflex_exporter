# WS2 Milestone A — Coverage Metrics Design Spec

**Date:** 2026-06-14
**Status:** Approved (design); pending implementation plan
**Source backlog:** `docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md` (WS2)
**Scope:** Implement the testable, non-replication, non-live-blocked WS2 coverage items
and harden Gen1 statistics collection against the all-or-nothing batch-query risk.

## Goal

Add the high/medium-value PowerFlex metrics from the WS2 backlog that are buildable and
testable without a replication cluster or live-cluster dtapi tracing, and make Gen1
statistics collection resilient so adding stats can never again break a whole cluster's
Gen1 metrics.

In scope (WS2 IDs): **WS2-09** (Sdt health/info), **WS2-11** (degraded/failed capacity +
the `degradedHealthy` split), **WS2-12** (Gen1 rebuild/rebalance capacity + job counts),
**WS2-13** (inventory counts), **WS2-15** (snapshot capacity), **WS2-17** (target/journaler
latency), **WS2-20** (device wear/temperature/error state).

## Non-goals

- **Replication** (WS2-01/02/03/14 — Sdr, RCG, ReplicationPair, replication stats) →
  Milestone B. New object types, replication-only value, untestable without a replicated
  cluster.
- **Gen2 live-verify-blocked** items (WS2-19 StorageNode `host_*`; the Gen2 half of
  WS2-12 remaining-capacity) → deferred until a live `--once --trace` confirms the dtapi
  metric names. Not in the spec.
- No shared Gen1/Gen2 fan-out abstraction. Decision: **mirror** Gen2's pattern in Gen1,
  keep the two fan-outs separate (less churn to working Gen2 code; accepted minor
  duplication).
- LOW-priority cosmetic items (WS2-04/05/06/07/08/10/16/18) are out of this milestone.

## Key findings that shape the work

- **Gen1 derivation is fully automatic** (`derivations.go:deriveSamples`): any stat name
  in `querySelectedStatistics.json` becomes a metric by suffix — `Bwc` → iops/bandwidth/
  io_size, `Latency` → latency (with op/direction via `splitDirection`), else → scalar
  gauge `pflex_<obj>_<snake(name)>`. So new Gen1 stats need **no collection code**, only
  validated stat names.
- **WS2-11 and WS2-15 are partly already collected.** `degradedFailedCapacityInKb`,
  `failedCapacityInKb`, `spareCapacityInKb`, `snapshotCapacityInKb`,
  `netSnapshotCapacityInKb` are already in `querySelectedStatistics.json` for
  System/StoragePool/ProtectionDomain and already auto-emit — they are undocumented with
  no dashboard panels. Their milestone work is docs + panels (+ the `degradedHealthy`
  split and `snapCapacityInUseInKb`, which are genuinely new).
- **Risk:** `querySelectedStatistics` is one batch call across all types. An invalid stat
  name for any type can fail the whole call and break **all** Gen1 metrics. Chosen
  mitigation: **defensive per-type isolation** (Component 1).
- **State metrics** come from instance properties via `state.go` (`deriveStateSamples` /
  `emitState`), not the statistics API. The `Instance` struct
  (`internal/models/instances.go`) currently has SDS/Device/SDC state fields but **not**
  `sdtState`, device `temperatureState`/`ssdEndOfLifeState`/`errorState`, or `numOf*`.

## Architecture — four components

### Component 1 — Gen1 per-type stats isolation (foundation)

Refactor `client.go:GetStatistics` (Gen1) from a single batch POST to one
`querySelectedStatistics` POST **per object type**, issued concurrently through Gen1's own
`errgroup` bounded by a new `gen1QueryConcurrency` constant (mirroring `v5QueryConcurrency`
/ `GetStatisticsV5`). Parse the embedded `querySelectedStatistics.json` (already grouped
by type) into per-type request bodies; merge the per-type responses into the existing
`*models.Statistics` aggregate. Failure semantics mirror `GetStatisticsV5`: a per-type
failure logs a warning and degrades (that type's samples are absent that cycle); the call
returns an error only when **every** type query fails. resty retry policy is unchanged
(429 + transport only; never 4xx/5xx — see CLAUDE.md).

This is intentionally NOT shared with Gen2's fan-out — the two remain separate
implementations of the same pattern.

### Component 2 — New Gen1 stats (no collection code)

Add the following to the per-type property lists in `querySelectedStatistics.json`, each
**validated against the pinned `docs/swagger/11231-4.5.5-Manager_v4-8-0.json` spec** as a
documented property of that type before inclusion:

- WS2-11: `degradedHealthyCapacityInKb` (System, StoragePool, ProtectionDomain — where the
  spec lists it).
- WS2-12: rebuild/rebalance **capacity** (`fwdRebuildCapacityInKb`, `bckRebuildCapacityInKb`,
  `normRebuildCapacityInKb`, `rebalanceCapacityInKb`) and **job counts**
  (`pendingMovingIn*Jobs`, `activeMovingIn*Jobs`, `pendingMovingOut*Jobs`, …) on
  ProtectionDomain/StoragePool — exact names per spec.
- WS2-15: `snapCapacityInUseInKb` (and `snapshotCapacityInKb` where missing on PD/SP).
- WS2-17: `targetReadLatency`, `targetWriteLatency`, `journalerReadLatency`,
  `journalerWriteLatency` (the `Latency` suffix auto-splits into op=`target`/`journaler`,
  direction=`read`/`write`).

Only names the spec confirms for the given type are added; any name not in the spec is
dropped to the Milestone-B/live-verify list rather than guessed.

### Component 3 — New state metrics

- **WS2-09 Sdt health/info (Gen2-only):** add `SdtState` (and any spec-listed Sdt state
  fields, e.g. `maintenanceState`/`mdmConnectionState`/`membershipState`) to
  `models.Instance`; add `sdtStateLabels`; add an `emitState(... TypeSdt,
  metricPrefix[TypeSdt], sdtStateLabels)` call in `deriveStateSamples` **guarded so it only
  runs for Gen2**; add the Sdt state strings to `healthSeverity`.
- **WS2-20 Device wear/temp/error (both gens):** add `TemperatureState`,
  `SsdEndOfLifeState`, `ErrorState` to `models.Instance`; extend `deviceStateLabels` to
  include them; add their state strings to `healthSeverity`. They fold into the existing
  `pflex_device_health` (worst-severity) and surface raw strings on `pflex_device_info`.

### Component 4 — Inventory counts + docs + dashboards

- **WS2-13 inventory counts:** add `numOf*` fields (e.g. `numOfSds`, `numOfVolumes`,
  `numOfDevices`, `numOfSnapshots`, `numOfMappedToAllVolumes`) to `models.Instance` for the
  types the spec documents them on (System, ProtectionDomain). Emit `pflex_<obj>_num_of_*`
  scalar gauges from the **API instance property** (authoritative; NOT `len()` of the
  relations map). This is a small new derivation path driven by instance properties (like
  state.go), distinct from the statistics API.
- **Docs:** update `docs/metrics.md` for every new metric AND the already-collected
  WS2-11/15 ones that were undocumented.
- **Dashboards:** add panels to the relevant `grafana/gen1/*` (and `grafana/gen2/*` for
  Sdt health) dashboards for the new metrics.

## Data flow

Unchanged shape: collection loop → per-cluster `collectCluster` → `collectGen1`/`collectGen2`
→ `buildSamplesGen1`/`buildSamplesGen2` + `deriveStateSamples` → `[]Sample` → SnapshotStore
→ Prom/OTLP readers. Component 1 changes only how `GetStatistics` fetches (per-type
concurrent vs batch); Components 2–4 add samples through the existing derivation/state
paths.

## Error handling

Component 1 is the error-handling story: per-type query failure → `log.Warnf` + degrade;
cluster Gen1 errors only if all types fail; an unknown stat name in one type cannot cascade
to others. State/inventory derivations skip objects whose label builder doesn't resolve
(existing `emitState` behavior). No change to the stats-path fallback in `collectCluster`.

## Testing

- **Isolation (Component 1):** extend the mock gateway (`client_test.go`) to serve per-type
  `querySelectedStatistics` requests; add a fault-injection test where ONE type returns 500
  and assert the other types' metrics still appear and the cluster Gen1 collection still
  succeeds. Add a test that all types failing returns an error.
- **Behavior preservation:** existing Gen1 collector tests must pass unchanged (same metric
  set emitted as before the refactor, given the same fixtures).
- **New metrics (Components 2–4):** extend fixtures (`instances*.json`, `statistics.json`)
  with the new stat/state/count fields; assert each new metric via **both** the Prometheus
  registry gather and the OTLP `ManualReader` (per existing convention).
- **Mixed-generation label-key consistency:** `TestMixedGenerationMetricsValid` must still
  pass (any Device additions keep the union label order).
- **Spec validation:** new stat/field names are checked against the pinned 4.5.5 spec
  (documented as part of the plan; a name absent from the spec is not added).
- Every slice gated by `make ci` (gofmt, vet, golangci-lint, `go test -race`, govulncheck)
  and the semgrep write-hook (restructure, never suppress).

## Phasing (one plan, four sequential slices)

1. **Isolation refactor** — behavior-preserving Gen1 per-type fan-out + isolation/fault
   tests. **Zero new metrics.** Lands the safety net first.
2. **New Gen1 stats** (WS2-11 split, WS2-12, WS2-15, WS2-17) — spec-validated names + tests.
3. **State metrics** (WS2-09 Sdt, WS2-20 device wear/temp) — model fields, builders,
   severity, tests.
4. **Inventory counts (WS2-13) + docs + dashboards** — new derivation + `metrics.md` +
   Grafana panels (incl. the previously-undocumented WS2-11/15 metrics).

## Release note

Because per-type isolation is the chosen safety net, a live-cluster `--once --debug` run is
**recommended but not required** before tagging the release. Spec-validation + the mock
fault-injection tests cover the previously-feared failure mode.

## Success criteria

- Gen1 statistics collection issues per-type queries and tolerates per-type failures
  (proven by the fault-injection test); existing Gen1 metric output is unchanged for
  unchanged fixtures.
- Each in-scope WS2 metric is emitted (asserted via Prometheus + OTLP), documented in
  `docs/metrics.md`, and has at least one Grafana panel.
- Every new stat/field name is confirmed present in the pinned 4.5.5 (or 5.0.0 for Gen2
  Sdt) spec.
- `make ci` green; `TestMixedGenerationMetricsValid` still passes; semgrep clean.
