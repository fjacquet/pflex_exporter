# Observability Completeness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add object health/state gauges, volume↔SDC mapping info metrics, and a starter Prometheus alert rule set to `pflex_exporter`, derived from data already fetched via `GET /api/instances`.

**Architecture:** A new derivation path turns *instance properties* into `Sample`s (today all samples come from *statistics* queries). `deriveStateSamples` reuses the existing per-type `labelBuilder` functions so the new series carry identical identity/parent labels — and the same Gen1/Gen2 union label-key sets — as the performance metrics. Output is appended to the existing per-cluster sample slice; the snapshot store and both export paths (Prometheus `/metrics`, OTLP push) are untouched.

**Tech Stack:** Go (standard lib + `encoding/json`), `prometheus/client_golang`, `go test` with a `httptest` mock PowerFlex gateway, Prometheus alerting rules (YAML), Docker Compose.

---

## Reference: verified facts this plan depends on

- Sample shape: `Sample{Name string; Labels []Label; Value float64}` (`internal/powerflex/metrics.go`). `Label{Name, Value string}`.
- `prometheus.go` `Collect` takes label **names from `samples[0]`** for each metric name — so **every sample sharing a metric name MUST have identical label keys in identical order**. Reusing the existing `labelBuilder`s guarantees this.
- Label builders live in `metrics.go`: `buildSdsLabels`, `buildStorageNodeLabels`, `buildDeviceLabelsGen1/Gen2` (both via the union `deviceLabels`), `buildSdcLabels` (identical both gens), `buildVolumeLabelsGen1/Gen2` (both via the union `volumeLabels`). Maps: `labelBuildersGen1`, `labelBuildersGen2`, `metricPrefix`.
- Wiring points: `buildSamplesGen1` and `buildSamplesGen2` in `internal/powerflex/collector.go` (each ends with `return samples`). `systemIDOf(in)` gives the System id.
- Generation constants in `gen.go`: `GenerationGen1 = "gen1"`, `GenerationGen2 = "gen2"`.
- `models.Instance` (`internal/models/instances.go`) currently models only a few fields; `ParseInstances` ignores unmodeled JSON. Type constants: `TypeSds`, `TypeStorageNode`, `TypeDevice`, `TypeSdc`, `TypeVolume`, etc.
- Test helpers (`internal/powerflex/collector_test.go`, `client_test.go`, `gen2_test.go`): `newMockGateway(t)` (defaults `instancesFixture="instances.json"`), `g.instancesFixture = "instances-gen2.json"`, `g.clientNamed(t, name)`, `assertSample(t, snap, name, matchMap, want)`, `assertLabel(t, snap, name, matchMap, labelName, want)`, `findSample`, `gatheredValue(mfs, name, matchMap)`.
- Confirmed capacity metric names — Gen1 StoragePool: `pflex_storagepool_capacity_in_use_in_kb`, `pflex_storagepool_max_capacity_in_kb`. Gen2 StoragePool: `pflex_storagepool_utilization_ratio` (already a 0–1 ratio). Latency: Gen1 `pflex_volume_latency`, Gen2 `pflex_volume_latency_microseconds`. Rebuild op present on storagepool/storagenode iops.
- Compose: `prometheus.yml` (repo root) is mounted to `/etc/prometheus/prometheus.yml` in both `docker-compose.yml` and `docker-compose.ghcr.yml`.

---

## File structure

- **Create** `internal/powerflex/state.go` — `deriveStateSamples`, severity table, `severityOf`, per-type state-label functions, `volumeMappingSamples`. One responsibility: instance-property → state/mapping samples.
- **Create** `internal/powerflex/state_test.go` — unit tests for `severityOf` + Gen1/Gen2 state/mapping sample tests.
- **Modify** `internal/models/instances.go` — add state fields + `MappedSdc` type to `Instance`.
- **Modify** `internal/powerflex/collector.go` — append `deriveStateSamples(...)` in `buildSamplesGen1`/`buildSamplesGen2`.
- **Modify** `internal/powerflex/testdata/instances.json` and `instances-gen2.json` — add state fields + `mappedSdcInfo`.
- **Modify** `internal/powerflex/gen2_test.go` — extend `TestMixedGenerationMetricsValid`.
- **Create** `deploy/prometheus/pflex.rules.yml` — alert rules.
- **Modify** `prometheus.yml`, `docker-compose.yml`, `docker-compose.ghcr.yml` — wire the rule file.
- **Modify** `docs/metrics.md`, **create** `docs/alerting.md`, **modify** `mkdocs.yml`, `CLAUDE.md` — docs.

---

## Stage A — Health/state gauges

### Task 1: Extend the instance model with state fields

**Files:**
- Modify: `internal/models/instances.go`
- Test: `internal/models/instances_test.go` (create if absent; otherwise add to the existing models test file `internal/models/config_test.go`'s package — use a new file `internal/models/instances_test.go`)

- [ ] **Step 1: Write the failing test**

Create `internal/models/instances_test.go`:

```go
package models

import "testing"

func TestParseInstancesStateFields(t *testing.T) {
	body := []byte(`{
		"System": {"id": "clu1", "name": "c1", "links": []},
		"sdsList": [{"id": "sds1", "name": "s1", "mdmConnectionState": "Connected",
			"membershipState": "Joined", "maintenanceState": "NoMaintenance", "links": []}],
		"deviceList": [{"id": "dev1", "name": "d1", "deviceState": "Normal", "links": []}],
		"volumeList": [{"id": "vol1", "name": "v1",
			"mappedSdcInfo": [{"sdcId": "sdc1", "sdcIp": "10.0.0.5"}], "links": []}]
	}`)

	in, _, err := ParseInstances(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sds := in.Get(TypeSds)[0]
	if sds.MdmConnectionState != "Connected" || sds.MembershipState != "Joined" || sds.MaintenanceState != "NoMaintenance" {
		t.Errorf("sds state = %q/%q/%q", sds.MdmConnectionState, sds.MembershipState, sds.MaintenanceState)
	}
	if got := in.Get(TypeDevice)[0].DeviceState; got != "Normal" {
		t.Errorf("device state = %q, want Normal", got)
	}
	vol := in.Get(TypeVolume)[0]
	if len(vol.MappedSdcInfo) != 1 || vol.MappedSdcInfo[0].SdcID != "sdc1" || vol.MappedSdcInfo[0].SdcIP != "10.0.0.5" {
		t.Errorf("mappedSdcInfo = %+v", vol.MappedSdcInfo)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/models/ -run TestParseInstancesStateFields -v`
Expected: FAIL — `sds.MdmConnectionState undefined` (compile error).

- [ ] **Step 3: Add the fields and type**

In `internal/models/instances.go`, add a `MappedSdc` type after the `Link` type:

```go
// MappedSdc is one entry of a Volume's mappedSdcInfo array: an SDC the volume is exposed to.
type MappedSdc struct {
	SdcID string `json:"sdcId"`
	SdcIP string `json:"sdcIp"`
}
```

Extend the `Instance` struct (add these fields after `MediaType`):

```go
	// Operational state (SDS / StorageNode; SDC reuses MdmConnectionState).
	MdmConnectionState string `json:"mdmConnectionState,omitempty"`
	MembershipState    string `json:"membershipState,omitempty"`
	MaintenanceState   string `json:"maintenanceState,omitempty"`
	// Device operational state.
	DeviceState string `json:"deviceState,omitempty"`
	// Volume -> SDC mappings (which hosts the volume is exposed to).
	MappedSdcInfo []MappedSdc `json:"mappedSdcInfo,omitempty"`
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/models/ -run TestParseInstancesStateFields -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/instances.go internal/models/instances_test.go
git commit -m "feat(models): parse SDS/device state and volume mappedSdcInfo"
```

---

### Task 2: Severity mapping helper

**Files:**
- Create: `internal/powerflex/state.go`
- Test: `internal/powerflex/state_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/powerflex/state_test.go`:

```go
package powerflex

import "testing"

func TestSeverityOf(t *testing.T) {
	cases := map[string]float64{
		"Connected":                0,
		"Joined":                   0,
		"NoMaintenance":            0,
		"Normal":                   0,
		"InMaintenance":            1,
		"JoinPending":              1,
		"NormalTesting":            1,
		"Disconnected":             2,
		"Decoupled":                2,
		"Failed":                   2,
		"":                         2, // missing signal is surfaced, not silently healthy
		"SomethingUnrecognized":    2,
	}
	for state, want := range cases {
		if got := severityOf(state); got != want {
			t.Errorf("severityOf(%q) = %v, want %v", state, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/powerflex/ -run TestSeverityOf -v`
Expected: FAIL — `undefined: severityOf`.

- [ ] **Step 3: Create state.go with the severity table**

Create `internal/powerflex/state.go`:

```go
package powerflex

import "github.com/fjacquet/pflex_exporter/internal/models"

// healthSeverity maps a PowerFlex operational-state string to a numeric severity used
// by the *_health gauges: 0 = healthy, 1 = degraded, 2 = failed/disconnected. Values
// not in this table (including the empty string) map to 2 via severityOf, so a missing
// or unrecognized signal is surfaced and alertable rather than silently healthy.
var healthSeverity = map[string]float64{
	// connection / membership / maintenance / device "good" states
	"Connected":     0,
	"Joined":        0,
	"NoMaintenance": 0,
	"Normal":        0,
	// degraded / transitional states
	"JoinPending":               1,
	"RemovePending":             1,
	"SetMaintenanceInProgress":  1,
	"InMaintenance":             1,
	"ExitMaintenanceInProgress": 1,
	"NormalTesting":             1,
	"DeviceInfoPending":         1,
	"Reserved":                  1,
	// failed / disconnected states
	"Disconnected": 2,
	"Decoupled":    2,
	"Failed":       2,
}

// severityOf returns the numeric severity for an operational-state string. Empty or
// unrecognized states return 2 (treated as failed/unknown).
func severityOf(state string) float64 {
	if sev, ok := healthSeverity[state]; ok {
		return sev
	}
	return 2
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestSeverityOf -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/powerflex/state.go internal/powerflex/state_test.go
git commit -m "feat(powerflex): add operational-state severity mapping"
```

---

### Task 3: deriveStateSamples — health + info, wired into the collector

**Files:**
- Modify: `internal/powerflex/state.go`
- Modify: `internal/powerflex/collector.go` (`buildSamplesGen1` and `buildSamplesGen2`, before each `return samples`)
- Modify: `internal/powerflex/testdata/instances.json`
- Test: `internal/powerflex/state_test.go`

- [ ] **Step 1: Add state fields to the Gen1 fixture**

In `internal/powerflex/testdata/instances.json`, replace the `sdsList`, `deviceList`, and `sdcList` blocks with state-bearing versions (keep the existing `links`):

```json
  "sdsList": [
    {
      "id": "sds1",
      "name": "sds-1",
      "mdmConnectionState": "Connected",
      "membershipState": "Joined",
      "maintenanceState": "NoMaintenance",
      "links": [
        { "rel": "/api/parent/relationship/protectionDomainId", "href": "/api/instances/ProtectionDomain::pd1" }
      ]
    }
  ],
  "deviceList": [
    {
      "id": "dev1",
      "name": "device-1",
      "deviceCurrentPathName": "/dev/sdb",
      "deviceState": "Normal",
      "links": [
        { "rel": "/api/parent/relationship/sdsId", "href": "/api/instances/Sds::sds1" },
        { "rel": "/api/parent/relationship/storagePoolId", "href": "/api/instances/StoragePool::sp1" }
      ]
    }
  ],
```

And the `sdcList` line:

```json
  "sdcList": [
    { "id": "sdc1", "name": null, "sdcIp": "10.0.0.5", "mdmConnectionState": "Connected", "links": [] }
  ]
```

- [ ] **Step 2: Write the failing test**

Add to `internal/powerflex/state_test.go`:

```go
import (
	"context"
	"testing"
	"time"
)

func gen1Snapshot(t *testing.T) *Snapshot {
	t.Helper()
	g := newMockGateway(t) // default instances.json -> gen1
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen1-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	return store.Load()
}

func TestGen1StateSamples(t *testing.T) {
	snap := gen1Snapshot(t)

	// Healthy fixture objects -> health 0.
	assertSample(t, snap, "pflex_sds_health", map[string]string{"sds_id": "sds1"}, 0)
	assertSample(t, snap, "pflex_device_health", map[string]string{"device_id": "dev1"}, 0)
	assertSample(t, snap, "pflex_sdc_health", map[string]string{"sdc_id": "sdc1"}, 0)

	// Info metric preserves raw state strings, value 1.
	assertSample(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, 1)
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "mdm_connection_state", "Connected")
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "membership_state", "Joined")
	assertLabel(t, snap, "pflex_sds_info", map[string]string{"sds_id": "sds1"}, "maintenance_state", "NoMaintenance")
	assertLabel(t, snap, "pflex_device_info", map[string]string{"device_id": "dev1"}, "device_state", "Normal")
	assertLabel(t, snap, "pflex_sdc_info", map[string]string{"sdc_id": "sdc1"}, "mdm_connection_state", "Connected")
}
```

Note: the `import` block must be merged with the existing one in `state_test.go` (which already imports `testing`). The final import set is `context`, `testing`, `time`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/powerflex/ -run TestGen1StateSamples -v`
Expected: FAIL — `sample pflex_sds_health... not found` (function not yet emitting).

- [ ] **Step 4: Implement deriveStateSamples in state.go**

Append to `internal/powerflex/state.go`:

```go
// stateLabelsFn returns the (info-label, raw-state-string) pairs for one object.
type stateLabelsFn func(o *models.Instance) []Label

func nodeStateLabels(o *models.Instance) []Label {
	return []Label{
		{"mdm_connection_state", o.MdmConnectionState},
		{"membership_state", o.MembershipState},
		{"maintenance_state", o.MaintenanceState},
	}
}

func deviceStateLabels(o *models.Instance) []Label {
	return []Label{{"device_state", o.DeviceState}}
}

func sdcStateLabels(o *models.Instance) []Label {
	return []Label{{"mdm_connection_state", o.MdmConnectionState}}
}

// deriveStateSamples turns instance operational-state properties into health gauges and
// info metrics for one cluster. It reuses the generation's label builders so the new
// series carry the same identity/parent labels (and union label-key sets) as the
// performance metrics. gen is GenerationGen1 or GenerationGen2.
func deriveStateSamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, gen string) []Sample {
	builders := labelBuildersGen1
	nodeType := models.TypeSds
	if gen == GenerationGen2 {
		builders = labelBuildersGen2
		nodeType = models.TypeStorageNode
	}

	var samples []Sample
	emitState(&samples, clusterName, systemID, in, rel, builders, nodeType, metricPrefix[nodeType], nodeStateLabels)
	emitState(&samples, clusterName, systemID, in, rel, builders, models.TypeDevice, metricPrefix[models.TypeDevice], deviceStateLabels)
	emitState(&samples, clusterName, systemID, in, rel, builders, models.TypeSdc, metricPrefix[models.TypeSdc], sdcStateLabels)
	return samples
}

// emitState appends a <prefix>_health gauge (worst severity across the object's state
// fields) and a <prefix>_info metric (raw state strings as labels, value 1) for every
// object of objType whose label builder resolves.
func emitState(samples *[]Sample, clusterName, systemID string, in *models.Instances, rel *models.Relations,
	builders map[string]labelBuilder, objType, prefix string, fn stateLabelsFn,
) {
	builder, ok := builders[objType]
	if !ok {
		return
	}
	for _, obj := range in.Get(objType) {
		base, ok := builder(clusterName, systemID, obj, in, rel)
		if !ok {
			continue
		}
		stateLabels := fn(obj)
		health := 0.0
		for _, sl := range stateLabels {
			if sev := severityOf(sl.Value); sev > health {
				health = sev
			}
		}
		*samples = append(*samples, Sample{Name: prefix + "_health", Labels: base, Value: health})

		info := make([]Label, 0, len(base)+len(stateLabels))
		info = append(info, base...)
		info = append(info, stateLabels...)
		*samples = append(*samples, Sample{Name: prefix + "_info", Labels: info, Value: 1})
	}
}
```

- [ ] **Step 5: Wire into the collector**

In `internal/powerflex/collector.go`, in `buildSamplesGen1`, change the final `return samples` to:

```go
	samples = append(samples, deriveStateSamples(clusterName, systemID, in, rel, GenerationGen1)...)
	return samples
```

In `buildSamplesGen2`, change the final `return samples` to:

```go
	samples = append(samples, deriveStateSamples(clusterName, systemID, in, rel, GenerationGen2)...)
	return samples
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestGen1StateSamples -v`
Expected: PASS.

- [ ] **Step 7: Run the full package to confirm no regressions**

Run: `go test ./internal/powerflex/`
Expected: PASS (existing collector/gen2 tests still green — new metrics are additive).

- [ ] **Step 8: Commit**

```bash
git add internal/powerflex/state.go internal/powerflex/collector.go internal/powerflex/testdata/instances.json internal/powerflex/state_test.go
git commit -m "feat(powerflex): emit health gauges and info metrics from instance state"
```

---

### Task 4: Gen2 state samples (StorageNode/Device/SDC)

**Files:**
- Modify: `internal/powerflex/testdata/instances-gen2.json`
- Test: `internal/powerflex/state_test.go`

- [ ] **Step 1: Add state fields to the Gen2 fixture**

In `internal/powerflex/testdata/instances-gen2.json`, add state fields to `storageNodeList`, `deviceList`, and `sdcList` (keep existing `links`):

`storageNodeList` entry gains:
```json
      "mdmConnectionState": "Connected",
      "membershipState": "Joined",
      "maintenanceState": "NoMaintenance",
```
`deviceList` entry gains:
```json
      "deviceState": "Normal",
```
`sdcList` becomes:
```json
  "sdcList": [
    { "id": "sdc1", "name": null, "sdcIp": "10.0.0.9", "mdmConnectionState": "Connected", "links": [] }
  ],
```

- [ ] **Step 2: Write the failing test**

Add to `internal/powerflex/state_test.go`:

```go
func TestGen2StateSamples(t *testing.T) {
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap := store.Load()

	assertSample(t, snap, "pflex_storagenode_health", map[string]string{"storage_node_id": "sn1"}, 0)
	assertSample(t, snap, "pflex_device_health", map[string]string{"device_id": "dev1"}, 0)
	assertSample(t, snap, "pflex_sdc_health", map[string]string{"sdc_id": "sdc1"}, 0)
	assertLabel(t, snap, "pflex_storagenode_info", map[string]string{"storage_node_id": "sn1"}, "membership_state", "Joined")
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/powerflex/ -run TestGen2StateSamples -v`
Expected: FAIL — samples not found (fixture lacked state until Step 1; if Step 1 is done it may already partially pass — the assertions confirm the Gen2 nodeType path).

- [ ] **Step 4: Verify the implementation already covers Gen2**

No code change needed — `deriveStateSamples` selects `models.TypeStorageNode` and `labelBuildersGen2` when `gen == GenerationGen2`. The test passing confirms it.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestGen2StateSamples -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/powerflex/testdata/instances-gen2.json internal/powerflex/state_test.go
git commit -m "test(powerflex): cover Gen2 health/info state samples"
```

---

## Stage B — Volume↔SDC mapping

### Task 5: pflex_volume_mapped_sdc info metric

**Files:**
- Modify: `internal/powerflex/state.go` (add `volumeMappingSamples`, call it from `deriveStateSamples`)
- Modify: `internal/powerflex/testdata/instances.json` and `instances-gen2.json` (add `mappedSdcInfo` to `vol1`)
- Test: `internal/powerflex/state_test.go`

- [ ] **Step 1: Add mappings to both fixtures**

In `internal/powerflex/testdata/instances.json`, the `volumeList` `vol1` entry gains a `mappedSdcInfo` (before `links`):
```json
      "mappedSdcInfo": [{ "sdcId": "sdc1", "sdcIp": "10.0.0.5" }],
```

In `internal/powerflex/testdata/instances-gen2.json`, the `volumeList` `vol1` entry gains:
```json
      "mappedSdcInfo": [{ "sdcId": "sdc1", "sdcIp": "10.0.0.9" }],
```

- [ ] **Step 2: Write the failing test**

Add to `internal/powerflex/state_test.go`:

```go
func TestVolumeMappedSdc(t *testing.T) {
	// Gen1
	snap1 := gen1Snapshot(t)
	assertSample(t, snap1, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, 1)
	assertLabel(t, snap1, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, "sdc_ip", "10.0.0.5")

	// Gen2
	g := newMockGateway(t)
	g.instancesFixture = "instances-gen2.json"
	store := NewSnapshotStore()
	c := NewCollector([]Client{g.clientNamed(t, "gen2-cluster")}, store, time.Second, 5*time.Second, nil)
	c.CollectOnce(context.Background())
	snap2 := store.Load()
	assertSample(t, snap2, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, 1)
	assertLabel(t, snap2, "pflex_volume_mapped_sdc",
		map[string]string{"volume_id": "vol1", "sdc_id": "sdc1"}, "sdc_ip", "10.0.0.9")
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/powerflex/ -run TestVolumeMappedSdc -v`
Expected: FAIL — `pflex_volume_mapped_sdc... not found`.

- [ ] **Step 4: Implement volumeMappingSamples**

Append to `internal/powerflex/state.go`:

```go
// volumeMappingSamples emits one pflex_volume_mapped_sdc info series (value 1) per
// volume→SDC mapping, correlating a volume with each host consuming it (no k8s needed).
// Volume identity/parent labels come from the generation's union volume label builder,
// then sdc_id and sdc_ip are appended in the same order for both generations.
func volumeMappingSamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, builders map[string]labelBuilder) []Sample {
	builder, ok := builders[models.TypeVolume]
	if !ok {
		return nil
	}
	var samples []Sample
	for _, vol := range in.Get(models.TypeVolume) {
		base, ok := builder(clusterName, systemID, vol, in, rel)
		if !ok {
			continue
		}
		for _, m := range vol.MappedSdcInfo {
			labels := make([]Label, 0, len(base)+2)
			labels = append(labels, base...)
			labels = append(labels, Label{"sdc_id", m.SdcID}, Label{"sdc_ip", m.SdcIP})
			samples = append(samples, Sample{Name: "pflex_volume_mapped_sdc", Labels: labels, Value: 1})
		}
	}
	return samples
}
```

Then call it from `deriveStateSamples` — add this line just before `return samples`:

```go
	samples = append(samples, volumeMappingSamples(clusterName, systemID, in, rel, builders)...)
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestVolumeMappedSdc -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/powerflex/state.go internal/powerflex/testdata/instances.json internal/powerflex/testdata/instances-gen2.json internal/powerflex/state_test.go
git commit -m "feat(powerflex): correlate volumes to consuming SDCs (pflex_volume_mapped_sdc)"
```

---

### Task 6: Mixed-generation label consistency

**Files:**
- Modify: `internal/powerflex/gen2_test.go` (`TestMixedGenerationMetricsValid`)

- [ ] **Step 1: Extend the mixed-generation test**

In `internal/powerflex/gen2_test.go`, inside `TestMixedGenerationMetricsValid`, after the existing assertions (before the closing brace), add checks that the new dual-generation metrics gather cleanly with consistent label keys:

```go
	// New state/mapping metrics emitted by BOTH generations must share consistent label
	// keys, or reg.Gather() above would already have failed. Spot-check presence.
	if _, ok := gatheredValue(mfs, "pflex_device_health", map[string]string{"cluster": "g1", "device_id": "dev1"}); !ok {
		t.Error("expected gen1 device health")
	}
	if _, ok := gatheredValue(mfs, "pflex_device_health", map[string]string{"cluster": "g2", "device_id": "dev1"}); !ok {
		t.Error("expected gen2 device health")
	}
	if _, ok := gatheredValue(mfs, "pflex_sdc_health", map[string]string{"cluster": "g1", "sdc_id": "sdc1"}); !ok {
		t.Error("expected gen1 sdc health")
	}
	if _, ok := gatheredValue(mfs, "pflex_volume_mapped_sdc", map[string]string{"cluster": "g1", "volume_id": "vol1", "sdc_id": "sdc1"}); !ok {
		t.Error("expected gen1 volume mapping")
	}
	if _, ok := gatheredValue(mfs, "pflex_volume_mapped_sdc", map[string]string{"cluster": "g2", "volume_id": "vol1", "sdc_id": "sdc1"}); !ok {
		t.Error("expected gen2 volume mapping")
	}
```

- [ ] **Step 2: Run the test**

Run: `go test ./internal/powerflex/ -run TestMixedGenerationMetricsValid -v`
Expected: PASS — and critically `reg.Gather()` does not error (proves `pflex_device_health`, `pflex_device_info`, `pflex_sdc_health`, `pflex_sdc_info`, `pflex_volume_mapped_sdc` carry identical label keys across generations).

- [ ] **Step 3: Run the whole package**

Run: `go test ./internal/powerflex/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/powerflex/gen2_test.go
git commit -m "test(powerflex): assert mixed-gen label consistency for state/mapping metrics"
```

---

## Stage C — Alert rules + docs

### Task 7: Starter Prometheus alert rules and compose wiring

**Files:**
- Create: `deploy/prometheus/pflex.rules.yml`
- Modify: `prometheus.yml`
- Modify: `docker-compose.yml`, `docker-compose.ghcr.yml`

- [ ] **Step 1: Create the rule file**

Create `deploy/prometheus/pflex.rules.yml`:

```yaml
# Starter alert rules for pflex_exporter. Thresholds are tunable defaults.
# NOTE: pflex_*_iops and *bandwidth* are already per-second gauges — aggregate with
# sum()/avg(), never rate().
groups:
  - name: pflex-health
    rules:
      - alert: PflexSdsUnhealthy
        expr: pflex_sds_health > 0 or pflex_storagenode_health > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "PowerFlex storage server unhealthy ({{ $labels.cluster }})"
          description: "SDS/StorageNode health={{ $value }} (1=degraded, 2=failed/disconnected). See pflex_sds_info / pflex_storagenode_info for the raw state."

      - alert: PflexDeviceFailed
        expr: pflex_device_health >= 2
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "PowerFlex device failed/unknown ({{ $labels.cluster }})"
          description: "Device {{ $labels.device_name }} health={{ $value }}. See pflex_device_info."

      - alert: PflexSdcDisconnected
        expr: pflex_sdc_health > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "PowerFlex SDC disconnected ({{ $labels.cluster }})"
          description: "SDC {{ $labels.sdc_name }} health={{ $value }}. See pflex_sdc_info."

  - name: pflex-capacity
    rules:
      - alert: PflexStoragePoolCapacityHighGen1
        expr: pflex_storagepool_capacity_in_use_in_kb / pflex_storagepool_max_capacity_in_kb > 0.85
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "PowerFlex (Gen1) storage pool over 85% capacity ({{ $labels.cluster }})"
          description: "Pool {{ $labels.storage_pool_name }} is {{ $value | humanizePercentage }} used."

      - alert: PflexStoragePoolCapacityCriticalGen1
        expr: pflex_storagepool_capacity_in_use_in_kb / pflex_storagepool_max_capacity_in_kb > 0.95
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "PowerFlex (Gen1) storage pool over 95% capacity ({{ $labels.cluster }})"
          description: "Pool {{ $labels.storage_pool_name }} is {{ $value | humanizePercentage }} used."

      - alert: PflexStoragePoolCapacityHighGen2
        expr: pflex_storagepool_utilization_ratio > 0.85
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "PowerFlex (Gen2) storage pool over 85% utilization ({{ $labels.cluster }})"
          description: "Pool {{ $labels.storage_pool_name }} utilization={{ $value | humanizePercentage }}."

  - name: pflex-activity
    rules:
      - alert: PflexRebuildActive
        expr: sum by (cluster, storage_pool_name) (pflex_storagepool_iops{op=~"fwdRebuild|bckRebuild|normRebuild|rebuild"}) > 0
        for: 10m
        labels:
          severity: info
        annotations:
          summary: "PowerFlex rebuild in progress ({{ $labels.cluster }})"
          description: "Rebuild IO active on pool {{ $labels.storage_pool_name }} for over 10m."

      - alert: PflexVolumeReadLatencyHigh
        expr: pflex_volume_latency{direction="read"} > 50 or pflex_volume_latency_microseconds{direction="read"} > 50000
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "PowerFlex volume read latency high ({{ $labels.cluster }})"
          description: "Volume {{ $labels.volume_name }} read latency over threshold."
```

- [ ] **Step 2: Reference the rule file in prometheus.yml**

In `prometheus.yml`, add a `rule_files` block after `global:`:

```yaml
global:
  scrape_interval: 15s

rule_files:
  - /etc/prometheus/pflex.rules.yml

scrape_configs:
  - job_name: powerflex
    metrics_path: /metrics
    static_configs:
      - targets: ["pflex_exporter:2112"]
```

- [ ] **Step 3: Mount the rule file in both compose files**

In `docker-compose.yml`, under the `prometheus` service `volumes:` (which already has the `prometheus.yml` mount), add:

```yaml
      - ./deploy/prometheus/pflex.rules.yml:/etc/prometheus/pflex.rules.yml:ro
```

Make the identical addition to the `prometheus` service `volumes:` in `docker-compose.ghcr.yml`.

- [ ] **Step 4: Validate the rule file syntax**

Run (uses the pinned Prometheus image; no local install needed):
```bash
docker run --rm -v "$PWD/deploy/prometheus/pflex.rules.yml:/r.yml:ro" --entrypoint promtool prom/prometheus:latest check rules /r.yml
```
Expected: `SUCCESS: ... rules found`. If Docker is unavailable, instead confirm valid YAML:
```bash
python3 -c "import yaml,sys; yaml.safe_load(open('deploy/prometheus/pflex.rules.yml')); print('yaml ok')"
```
Expected: `yaml ok`.

- [ ] **Step 5: Commit**

```bash
git add deploy/prometheus/pflex.rules.yml prometheus.yml docker-compose.yml docker-compose.ghcr.yml
git commit -m "feat(deploy): ship starter Prometheus alert rules and wire into compose"
```

---

### Task 8: Documentation and final CI gate

**Files:**
- Modify: `docs/metrics.md`
- Create: `docs/alerting.md`
- Modify: `mkdocs.yml`
- Modify: `CLAUDE.md`

- [ ] **Step 1: Document the new metrics in docs/metrics.md**

In `docs/metrics.md`, under the "Health & meta metrics" table, add the new rows and a new subsection after the table:

Add rows to the existing table:
```markdown
| `pflex_<obj>_health` | object identity/parent labels | Operational severity: `0`=healthy, `1`=degraded, `2`=failed/disconnected/unknown. Emitted for SDS/StorageNode, Device, SDC. |
| `pflex_<obj>_info` | identity labels + raw state strings | Always `1`; carries raw PowerFlex state strings (`mdm_connection_state`, `membership_state`, `maintenance_state`, `device_state`). |
| `pflex_volume_mapped_sdc` | volume identity/parent labels + `sdc_id`, `sdc_ip` | Always `1`; one series per volume→SDC mapping, correlating a volume with each host consuming it. |
```

Then add this subsection:
```markdown
### Operational state

State gauges are derived from object properties in `GET /api/instances` (not from the
statistics API). The `*_health` value is the **worst severity** across the object's state
fields; the matching `*_info` metric preserves the raw strings as labels. A missing or
unrecognized state maps to `2` (unknown), so a lost signal is surfaced rather than hidden.

Expected state fields are present on PowerFlex 4.5+ (Gen1) and 5.x (Gen2). Alert on
`pflex_<obj>_health > 0`; join to a host with
`pflex_volume_mapped_sdc` (e.g. `pflex_volume_iops * on(volume_id) group_left(sdc_id) pflex_volume_mapped_sdc`).
```

- [ ] **Step 2: Create docs/alerting.md**

Create `docs/alerting.md`:

```markdown
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
```

- [ ] **Step 3: Add the alerting page to mkdocs nav**

In `mkdocs.yml`, add `alerting.md` to the `nav:` list adjacent to the `metrics.md`/`dashboards.md` entries (match the existing nav style, e.g. `- Alerting: alerting.md`).

- [ ] **Step 4: Update CLAUDE.md "Adding metrics or object types"**

In `CLAUDE.md`, in the "Adding metrics or object types" section, append a bullet:

```markdown
- **Operational state (both gens):** state/health metrics come from a second derivation path in `state.go` (`deriveStateSamples`), driven by *instance properties* rather than the statistics API. Add a state field to `models.Instance`, a `stateLabelsFn`, an `emitState` call, and a severity entry in `healthSeverity`. `pflex_volume_mapped_sdc` is emitted by `volumeMappingSamples` from `Volume.mappedSdcInfo`. Metrics emitted by both generations (Device, SDC, Volume) must reuse the union label builders.
```

- [ ] **Step 5: Build the docs site (strict)**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: build succeeds with no warnings (strict fails on broken nav/links).

- [ ] **Step 6: Run the full CI gate**

Run: `make ci`
Expected: gofmt clean, `go vet` clean, golangci-lint clean, `go test -race` PASS, govulncheck clean.

(Note: a Semgrep scan runs on every file write via a hook and blocks on findings. Test handlers already write fixtures via the `writeBytes(io.Writer, …)` helper — no new ResponseWriter writes are introduced here.)

- [ ] **Step 7: Commit**

```bash
git add docs/metrics.md docs/alerting.md mkdocs.yml CLAUDE.md
git commit -m "docs: document health/state metrics, volume↔SDC mapping, and alert rules"
```

---

## Self-review notes

- **Spec coverage:** A (Task 1–4: struct, severity, health+info, Gen1+Gen2) ✓; B (Task 5: `pflex_volume_mapped_sdc`) ✓; C (Task 7 rules, Task 8 docs) ✓; label-consistency guard (Task 6) ✓; docs lockstep (Task 8) ✓. Capacity is reused, not re-added, per spec.
- **No placeholders:** every code/edit step shows concrete content; alert expressions use verified metric names; capacity covered for both generations.
- **Type consistency:** `severityOf`, `healthSeverity`, `stateLabelsFn`, `emitState`, `deriveStateSamples`, `volumeMappingSamples`, `models.MappedSdc` are named identically wherever referenced. `deriveStateSamples` is called in both `buildSamplesGen1`/`buildSamplesGen2` with the matching generation constant.
- **Constraints honored:** union label builders reused (Prometheus label-key consistency); no `rate()` guidance carried into rules; no new auth/retry behavior; no ResponseWriter writes; no Dockerfile change.
```
