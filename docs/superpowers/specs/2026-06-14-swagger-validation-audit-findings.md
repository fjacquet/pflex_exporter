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

## WS1 — Correctness audit

Scope: endpoints, request payloads, query params, and response field names the
exporter USES, cross-checked against the in-scope Dell PowerFlex swagger specs
(`11227-*` auth, `11231-4.5.5` Gen1 Block API, `11231-5.0.0` Gen2 Block Storage
API). Trust hierarchy applied: **passing tests + CLAUDE.md + live-cluster docs >
spec**. NO code changed in this task.

### Endpoint contracts

| ID | Endpoint | Aspect | Code (file:line) | Spec | Verdict | Recommendation |
|----|----------|--------|------------------|------|---------|----------------|
| WS1-01 | POST /rest/auth/login | path + method | client.go:37, auth.go:74-78 | present (11227-4.5.5 & 5.0.0) | MATCH | none |
| WS1-02 | POST /rest/auth/login | request body `username`/`password` | auth.go:77 | schema `login-credentials` example `{username,password}` | MATCH | none |
| WS1-03 | POST /rest/auth/login | response `access_token`/`refresh_token` read | auth.go:21-22,86-98 | 200 fields include `access_token`, `refresh_token` | MATCH | none |
| WS1-04 | POST /rest/auth/update-token | path + method | client.go:38, auth.go:104-109 | present (both 11227) | MATCH | none |
| WS1-05 | POST /rest/auth/update-token | request body `refresh_token` | auth.go:108 | schema `update-token-request` example `{refresh_token}` | MATCH | none |
| WS1-06 | POST /rest/auth/update-token | response `access_token`/`refresh_token` read | auth.go:118-132 | 200 fields include both | MATCH | none |
| WS1-07 | GET /api/instances (aggregate, all types) | path + method | client.go:31,149-154 | NOT in in-scope spec (spec only has per-type `GET /api/instances/{Type}::{id}` and `GET /api/types/{Type}/instances`) | MISMATCH(reality-wins) | none — endpoint is a real PowerFlex aggregate route; exercised by mock gateway + live `--once`. Spec omission, not a code bug. |
| WS1-08 | POST /api/instances/querySelectedStatistics (multi-type batch) | path + method | client.go:32,158-163 | NOT in in-scope spec (spec only has per-type `POST /api/types/{Type}/instances/action/querySelectedStatistics`) | MISMATCH(reality-wins) | none — the aggregate multi-type batch route is real PowerFlex; embedded `querySelectedStatistics.json` `selectedStatisticsList[]{type,allIds,properties[]}` is the documented batch form. Tests + live `--once` confirm. |
| WS1-09 | Gen1 request body shape (`selectedStatisticsList`) | request payload | querySelectedStatistics.json | per-type spec ops have empty/abstract requestBody (`{}`) | MISMATCH(reality-wins) | none — spec does not document the batch body; the `selectedStatisticsList` form is the live contract (passing `TestGen1*`). |
| WS1-10 | POST /dtapi/rest/v1/metrics/query | path + method | client.go:33,175-247 | present (11231-5.0.0) | MATCH | none |
| WS1-11 | dtapi `resource_type` one-per-call | request payload | client.go:194-206 (per-type fan-out) | requestBody `required:[resource_type]`, single string | MATCH | none |
| WS1-12 | dtapi `metrics` field form | request payload | client.go:206 (JSON array) | requestBody `metrics: {type: string}` (comma-separated) | **MISMATCH(reality-wins)** | **NEVER change.** See WS1-13 below. |
| WS1-13 | GET /api/instances (Gen2) | path + method | client.go:31,149-154 | same omission as WS1-07 (Gen2 spec only per-type) | MISMATCH(reality-wins) | none — aggregate route real on Gen2 too; mock + live confirm. |

### Notable

- **WS1-12 / dtapi `metrics` field: spec = comma-separated STRING, reality =
  JSON ARRAY → MISMATCH(reality-wins). DO NOT change.** Raw spec
  (`docs/swagger/11231-5.0.0.json` → `/dtapi/rest/v1/metrics/query` requestBody)
  declares `"metrics": {"type": "string", "description": "A list of specific
  metrics to retrieve, separated by commas."}`. The live dtapi returns an
  instant **HTTP 500** for a string and accepts only a JSON array — matching
  Dell's reference `siocli`/`sio_sdk`. Code correctly sends an array
  (`client.go:206`). Sending the spec's string reintroduces the **v0.6.2 HTTP
  500 regression**. Cited in `CLAUDE.md:32` ("The request's `metrics` field is a
  **JSON array** … a comma-separated string returns an instant HTTP 500 … the
  PowerFlex 5.0 PDF documents a string, but it is wrong — trusting it shipped a
  regression in v0.6.2") and reinforced at `CLAUDE.md:46` (no-5xx-retry
  rationale). Guarded by `client_test.go:385` (asserts array form). **Recommend
  NEVER changing.**

- **dtapi response envelope (`resources[]{id,metrics[]{name,values[]}}`) is not
  in the in-scope spec** → MISMATCH(reality-wins). The spec's `MetricsQuery`
  200 schema is an abstract `oneOf` of per-type `*-metrics` objects keyed by
  flat metric names; it does NOT define the `resources` array envelope the code
  unmarshals (`statistics_v5.go:28-40`). The test fixture
  (`testdata/statistics-v5.json`, served via `client_test.go:150` as
  `{"resources": <per-type array>}`) and live `--trace` confirm the real shape.
  No code change.

- The dtapi 200 field list extracted from the spec contains obvious typos
  (`avg_host_to_write_latency`, `idevice_local_read_bandwidthd`,
  `idevice_remote_write_bandwidthd`, `thin_provisioning_ration`,
  `Logical_owned`, `raw_rebalance`). These are spec-side noise; the code's
  `v5Metrics` table uses the correct names. Not actionable.

### Struct field cross-check

Fields OUR CODE reads (focused on identity / state / stat-counter fields it
actually unmarshals), checked against the in-scope spec:

**Confirmed by spec (MATCH):**
- Identity/relations: `id`, `name`, `links`, `href`, `rel` — all present in
  Gen1 spec (`11231-4.5.5`). (`instances.go:26-27,42`)
- Gen1 stat counters: `primaryReadBwc` (Bwc family), `numSeconds`,
  `totalWeightInKb`, and the API's misspelled **`numOccured`** (single 'r') —
  all present in Gen1 spec. Code reads both `numOccured` and `numOccurred`
  defensively (`statistics.go:14-15`); the spec confirms the single-'r' form is
  the real one. MATCH.
- Gen2 state/identity: `id`, `name`, `dataLayout`, `mdmConnectionState`,
  `maintenanceState`, `deviceState`, `membershipState`, `mediaType`,
  `volumeType`, `sdcId`, `sdcIp` — all present in Gen2 spec (`11231-5.0.0`).
- Auth: `access_token`, `refresh_token` — present in both 11227 specs.

**Read by code, NOT in in-scope spec (trust-rule applied):**
- `mappedSdcInfo` (Volume mapping → `pflex_volume_mapped_sdc`) — absent from
  both 11231 specs. Present in live test fixtures
  (`testdata/instances.json`, `instances-gen2.json`, `instances-unhealthy.json`)
  and exercised by passing tests → **MISMATCH(reality-wins)**, not a bug.
- `deviceCurrentPathName` (Device label) — absent from both 11231 specs; present
  in the same three fixtures + passing tests → **MISMATCH(reality-wins)**, not a
  bug.
- Gen2 `links`/`href`/`rel` for the relations graph — the aggregate Gen2
  `/api/instances` response (with link hrefs) is the WS1-07/WS1-13 undocumented
  route, so these too fall under reality-wins via that endpoint.

**No BUG-classified findings in WS1.** Every code/spec discrepancy is backed by
passing tests, fixtures, or CLAUDE.md → all MISMATCH(reality-wins). Nothing feeds
Task 10 from this work-stream.

### Verdict counts (WS1)

- MATCH: WS1-01..06, WS1-10, WS1-11 + struct fields (id/name/links/href/rel,
  Bwc/numSeconds/totalWeightInKb/numOccured, Gen2 state fields, auth tokens).
- MISMATCH(reality-wins): WS1-07, WS1-08, WS1-09, WS1-12, WS1-13 + dtapi
  response envelope + `mappedSdcInfo` + `deviceCurrentPathName` (8 distinct
  discrepancies, all spec-side, recommend no code change).
- BUG: 0.
