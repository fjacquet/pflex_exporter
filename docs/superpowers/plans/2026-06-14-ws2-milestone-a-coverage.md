# WS2 Milestone A — Coverage Metrics Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the testable WS2 coverage metrics (WS2-09/11/12/13/15/17/20) and make Gen1 statistics collection resilient by issuing one `querySelectedStatistics` query per object type so a single bad stat name can never break a whole cluster's Gen1 metrics.

**Architecture:** Mirror the existing Gen2 `GetStatisticsV5` per-type concurrent fan-out (errgroup + `SetLimit`, partial-failure tolerant) in Gen1's `GetStatistics`. New Gen1 stats are added as spec-validated names to `querySelectedStatistics.json` (derivation is automatic). New state metrics extend `state.go` + `models.Instance`. Inventory counts are a small instance-property derivation. Each metric is documented and dashboarded.

**Tech Stack:** Go (`internal/powerflex`, `internal/models`), `golang.org/x/sync/errgroup`, Prometheus client_golang + OTLP `ManualReader` tests, Grafana JSON, MkDocs.

**Working branch:** `feat/ws2-milestone-a` (already created; spec at `docs/superpowers/specs/2026-06-14-ws2-milestone-a-coverage-design.md`).

**Hard constraints (load-bearing — do not violate):**
- resty retry stays 429 + transport only; never add 4xx/5xx retry (CLAUDE.md).
- Gen2 dtapi `metrics` field stays a JSON array; do not touch `GetStatisticsV5` behavior.
- Prometheus label-key consistency: any `Device`/`Volume` change keeps the union label order (`deviceLabels`/`volumeLabels`); `TestMixedGenerationMetricsValid` must pass.
- A semgrep hook runs on every write and blocks on findings; inline `// nosemgrep` is NOT honored — restructure (e.g. test handlers write via the `writeBytes(io.Writer, …)` helper).
- Every new stat/field name MUST be confirmed present in the pinned spec before use:
  Gen1 → `docs/swagger/11231-4.5.5-Manager_v4-8-0.json`; Gen2 Sdt → `docs/swagger/11231-5.0.0.json`. Use `python3 scripts/audit/extract_swagger.py <spec> | grep -i <name>` or `grep -o '"<name>"' <spec>`.

---

## File Structure

**Modify:**
- `internal/powerflex/client.go` — refactor `GetStatistics` to per-type fan-out; add `gen1QueryConcurrency`.
- `internal/powerflex/query.go` — add a helper to split the embedded `queryStatsBody` into per-type request bodies.
- `internal/powerflex/querySelectedStatistics.json` — add spec-validated WS2-12/15/17/11 stat names.
- `internal/models/statistics.go` — add a `Merge` method on `*Statistics` (combine per-type responses).
- `internal/models/instances.go` — add `SdtState`, device `TemperatureState`/`SsdEndOfLifeState`/`ErrorState`, and `numOf*` fields.
- `internal/powerflex/state.go` — add `sdtStateLabels`, extend `deviceStateLabels`, Gen2-guarded Sdt `emitState`, `healthSeverity` entries.
- `internal/powerflex/metrics.go` — inventory-count derivation (or new small file `inventory.go`).
- `internal/powerflex/collector.go` — wire the inventory derivation into `buildSamplesGen1`/`buildSamplesGen2`.
- `internal/powerflex/client_test.go` — mock gateway: per-type `querySelectedStatistics` handling + fault injection.
- `internal/powerflex/testdata/*.json` — extend fixtures with new stat/state/count fields.
- `docs/metrics.md`, `docs/dashboards.md`, `grafana/gen1/*`, `grafana/gen2/06-storage-node.json`.

**Create:**
- `docs/adr/0002-gen1-per-type-statistics-isolation.md`
- `docs/adr/0003-reality-outranks-spec-dtapi-contract.md`
- `internal/powerflex/inventory.go` (if cleaner than putting it in metrics.go)

---

## Slice 1 — Isolation refactor + ADRs

### Task 1: Write the two ADRs

**Files:**
- Create: `docs/adr/0002-gen1-per-type-statistics-isolation.md`
- Create: `docs/adr/0003-reality-outranks-spec-dtapi-contract.md`

- [ ] **Step 1: Write ADR 0002**

Create `docs/adr/0002-gen1-per-type-statistics-isolation.md`:

```markdown
# ADR 0002 — Gen1 per-type statistics isolation

## Status
Accepted (2026-06-14)

## Context
Gen1 statistics are fetched with a single `POST /api/instances/querySelectedStatistics`
whose `selectedStatisticsList` batches every object type. If one type lists a stat name
the API rejects, the whole call can fail — taking down *all* Gen1 metrics for that cluster.
The WS2 coverage work adds new stat names, increasing this risk. Gen2 already avoids the
analogous problem because the dtapi endpoint forces one `resource_type` per call, fanned
out concurrently (`GetStatisticsV5`).

## Decision
Refactor Gen1 `GetStatistics` to issue one `querySelectedStatistics` call **per object
type**, concurrently, bounded by `gen1QueryConcurrency`, merging the per-type responses.
A per-type failure logs a warning and degrades (that type absent for the cycle); the call
errors only if **every** type fails — mirroring `GetStatisticsV5`. The two fan-outs are
kept **separate** (mirror, not shared abstraction) to avoid churning working Gen2 code.

## Consequences
- A bad stat name for one type can no longer break the others.
- Gen1 now issues N calls/cluster/cycle instead of 1 (N = number of types). Bounded
  concurrency keeps this within `collection.timeout`.
- Future Gen1 stat additions are low-risk.
- Minor duplication of the errgroup fan-out shape between Gen1 and Gen2 (accepted).
```

- [ ] **Step 2: Write ADR 0003**

Create `docs/adr/0003-reality-outranks-spec-dtapi-contract.md`:

```markdown
# ADR 0003 — Reality outranks spec; the dtapi metrics-array contract

## Status
Accepted (2026-06-14)

## Context
The 2026-06-14 swagger validation audit confirmed Dell's PowerFlex 5.0 spec documents the
Gen2 dtapi `/dtapi/rest/v1/metrics/query` request `metrics` field as a comma-separated
string. The live dtapi returns an instant HTTP 500 for a string and accepts only a JSON
array. Trusting the spec shipped the v0.6.2 regression. Several other code behaviors (the
aggregate `/api/instances` routes, response envelopes, the `numOccured` misspelling) also
diverge from or extend the documented spec but match the live API.

## Decision
Adopt an explicit trust hierarchy: **passing tests + documented live-cluster behavior +
maintainer notes outrank the swagger spec.** On conflict, prefer reality and record the
spec discrepancy rather than "fixing" code to the spec. Concretely:
- The dtapi `metrics` field is sent as a JSON array. Never change it to a string.
- resty retry is limited to 429 + transport errors — never 4xx/5xx — so a deterministic
  5xx (e.g. a malformed query) surfaces instead of being buried under a timeout.

## Consequences
- Audits flag spec-vs-reality conflicts as notes, not bugs (see the audit findings doc).
- Pinned specs live in `docs/swagger/` as the reference; the audit is reproducible.
- Contributors must not "correct" the array form or add a 5xx retry clause.
```

- [ ] **Step 3: Commit**

```bash
git add docs/adr/0002-gen1-per-type-statistics-isolation.md docs/adr/0003-reality-outranks-spec-dtapi-contract.md
git commit -m "docs(adr): 0002 Gen1 per-type isolation, 0003 reality-outranks-spec"
```

### Task 2: Per-type request splitter

**Files:**
- Modify: `internal/powerflex/query.go`
- Test: `internal/powerflex/query_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/powerflex/query_test.go`:

```go
package powerflex

import "testing"

func TestGen1PerTypeBodies(t *testing.T) {
	bodies, err := gen1PerTypeBodies()
	if err != nil {
		t.Fatalf("gen1PerTypeBodies: %v", err)
	}
	// The embedded querySelectedStatistics.json defines these types.
	for _, want := range []string{"System", "Sds", "Sdc", "Volume", "StoragePool", "Device", "ProtectionDomain"} {
		body, ok := bodies[want]
		if !ok {
			t.Errorf("missing per-type body for %q", want)
			continue
		}
		// Each body must be a selectedStatisticsList with exactly one entry of that type.
		if got := countSelectedTypes(t, body, want); got != 1 {
			t.Errorf("body for %q: want exactly 1 entry of that type, got %d", want, got)
		}
	}
}

// countSelectedTypes parses body and returns how many selectedStatisticsList entries have type==typ.
func countSelectedTypes(t *testing.T, body []byte, typ string) int {
	t.Helper()
	var doc struct {
		SelectedStatisticsList []struct {
			Type string `json:"type"`
		} `json:"selectedStatisticsList"`
	}
	if err := jsonUnmarshal(body, &doc); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	n := 0
	for _, e := range doc.SelectedStatisticsList {
		if e.Type == typ {
			n++
		}
	}
	return n
}
```

Note: use the standard library directly — replace `jsonUnmarshal` with `json.Unmarshal` and add the `encoding/json` import. (The helper name is only to keep this snippet self-contained; write real `json.Unmarshal`.)

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/powerflex/ -run TestGen1PerTypeBodies -v`
Expected: FAIL — `gen1PerTypeBodies` undefined.

- [ ] **Step 3: Implement the splitter**

Add to `internal/powerflex/query.go` (keep the existing `//go:embed` + `queryStatsBody`):

```go
import "encoding/json"

// gen1SelectedStatistics mirrors the embedded querySelectedStatistics.json shape.
type gen1SelectedStatistics struct {
	SelectedStatisticsList []json.RawMessage `json:"selectedStatisticsList"`
}

// gen1PerTypeBodies splits the embedded querySelectedStatistics.json into one request body
// per object type, each a selectedStatisticsList with a single entry. This lets Gen1 stats
// be fetched per type (one bad type cannot fail the others). Keyed by the entry's "type".
func gen1PerTypeBodies() (map[string][]byte, error) {
	var doc gen1SelectedStatistics
	if err := json.Unmarshal(queryStatsBody, &doc); err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(doc.SelectedStatisticsList))
	for _, entry := range doc.SelectedStatisticsList {
		var meta struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(entry, &meta); err != nil {
			return nil, err
		}
		body, err := json.Marshal(map[string]any{
			"selectedStatisticsList": []json.RawMessage{entry},
		})
		if err != nil {
			return nil, err
		}
		out[meta.Type] = body
	}
	return out, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestGen1PerTypeBodies -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/powerflex/query.go internal/powerflex/query_test.go
git commit -m "feat(powerflex): split Gen1 querySelectedStatistics into per-type bodies"
```

### Task 3: Statistics.Merge

**Files:**
- Modify: `internal/models/statistics.go`
- Test: `internal/models/statistics_test.go` (create or extend)

- [ ] **Step 1: Write the failing test**

Create/extend `internal/models/statistics_test.go`:

```go
package models

import (
	"encoding/json"
	"testing"
)

func TestStatisticsMerge(t *testing.T) {
	sys, _ := ParseStatistics([]byte(`{"System":{"maxCapacityInKb":10}}`))
	sds, _ := ParseStatistics([]byte(`{"Sds":{"sds1":{"primaryReadBwc":{"numOccured":1,"numSeconds":1,"totalWeightInKb":2}}}}`))
	agg := &Statistics{ByType: map[string]map[string]StatMap{}}
	agg.Merge(sys)
	agg.Merge(sds)
	if _, ok := agg.System["maxCapacityInKb"]; !ok {
		t.Error("merged System missing maxCapacityInKb")
	}
	if agg.Object("Sds", "sds1") == nil {
		t.Error("merged ByType missing Sds/sds1")
	}
	_ = json.RawMessage{}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/models/ -run TestStatisticsMerge -v`
Expected: FAIL — `Merge` undefined.

- [ ] **Step 3: Implement Merge**

Add to `internal/models/statistics.go`:

```go
// Merge folds another parsed Statistics into s: System stats overwrite (last wins) and
// per-type object maps are merged by type then object ID. Used to combine the per-type
// querySelectedStatistics responses of the Gen1 fan-out into one aggregate.
func (s *Statistics) Merge(other *Statistics) {
	if other == nil {
		return
	}
	if other.System != nil {
		if s.System == nil {
			s.System = make(StatMap, len(other.System))
		}
		for k, v := range other.System {
			s.System[k] = v
		}
	}
	if s.ByType == nil {
		s.ByType = make(map[string]map[string]StatMap)
	}
	for typ, byID := range other.ByType {
		if s.ByType[typ] == nil {
			s.ByType[typ] = make(map[string]StatMap, len(byID))
		}
		for id, sm := range byID {
			s.ByType[typ][id] = sm
		}
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/models/ -run TestStatisticsMerge -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/models/statistics.go internal/models/statistics_test.go
git commit -m "feat(models): add Statistics.Merge for Gen1 per-type fan-out"
```

### Task 4: Refactor GetStatistics to per-type fan-out

**Files:**
- Modify: `internal/powerflex/client.go:157-164` (the `GetStatistics` method) and add `gen1QueryConcurrency`.
- Test: `internal/powerflex/client_test.go` (mock gateway per-type handling + fault injection)

- [ ] **Step 1: Update the mock gateway to serve per-type querySelectedStatistics**

In `internal/powerflex/client_test.go`, find the handler for `statisticsPath`
(`/api/instances/querySelectedStatistics`). It currently returns the whole
`statistics.json`. Change it to: read the request body, determine the single entry's
`type`, and return only that type's slice of the fixture. Add a fault-injection switch.
Mirror the existing `failStatsV5` field. Concretely add to the mock gateway struct:

```go
	failStatsType string // when set, querySelectedStatistics for this type returns HTTP 500
```

And replace the querySelectedStatistics handler body with (write the fixture via the
existing `writeBytes` helper to satisfy semgrep):

```go
	// Parse which single type this per-type request asks for.
	var req struct {
		SelectedStatisticsList []struct {
			Type string `json:"type"`
		} `json:"selectedStatisticsList"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	typ := ""
	if len(req.SelectedStatisticsList) == 1 {
		typ = req.SelectedStatisticsList[0].Type
	}
	if typ != "" && typ == g.failStatsType {
		w.WriteHeader(http.StatusInternalServerError)
		writeBytes(w, []byte(`{"message":"injected failure"}`))
		return
	}
	// Return only this type's portion of the statistics fixture.
	full := g.readFixture("statistics.json") // existing fixture loader; adapt to actual helper name
	var all map[string]json.RawMessage
	_ = json.Unmarshal(full, &all)
	out := map[string]json.RawMessage{}
	if seg, ok := all[typ]; ok {
		out[typ] = seg
	}
	b, _ := json.Marshal(out)
	writeBytes(w, b)
```

Adapt `g.readFixture(...)` to whatever the existing fixture-reading helper is (discover via `grep -n "statistics.json\|readFixture\|os.ReadFile\|testdata" internal/powerflex/client_test.go`).

- [ ] **Step 2: Write the failing isolation test**

Add to `internal/powerflex/client_test.go`:

```go
func TestGen1StatsPerTypeIsolation(t *testing.T) {
	g := newMockGateway(t)
	g.failStatsType = "Device" // one type's query fails
	client := g.clientNamed(t, "c1")

	stats, err := client.GetStatistics(context.Background())
	if err != nil {
		t.Fatalf("GetStatistics should degrade, not error, on one failed type: %v", err)
	}
	// A non-failed type still has data.
	if stats.System == nil || len(stats.System) == 0 {
		t.Error("System stats missing despite only Device failing")
	}
	// The failed type degrades to absent (no panic, no whole-call failure).
	if _, ok := stats.ByType["Device"]; ok && len(stats.ByType["Device"]) > 0 {
		t.Error("Device stats should be absent when its query was injected to fail")
	}
}
```

Adapt `newMockGateway`/`clientNamed` to the actual helper names (see existing tests).

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/powerflex/ -run TestGen1StatsPerTypeIsolation -v`
Expected: FAIL — current `GetStatistics` sends one batch request (no per-type behavior; with the new mock it returns only the matched single type or empties), so it won't behave as asserted until refactored.

- [ ] **Step 4: Refactor GetStatistics**

Replace `internal/powerflex/client.go` `GetStatistics` (lines ~157-164) with a per-type
fan-out mirroring `GetStatisticsV5`. Add the constant near `v5QueryConcurrency`:

```go
// gen1QueryConcurrency bounds how many per-type querySelectedStatistics calls run at once.
// Gen1 fans out one call per object type so a single rejected stat name degrades only that
// type instead of failing the whole cluster's Gen1 collection (ADR 0002).
const gen1QueryConcurrency = 8
```

```go
// GetStatistics fetches Gen1 statistics by querying querySelectedStatistics once per object
// type, concurrently. A failed per-type query is logged and skipped (graceful
// degradation); the call errors only when every type query fails. See ADR 0002.
func (c *ClusterClient) GetStatistics(ctx context.Context) (*models.Statistics, error) {
	bodies, err := gen1PerTypeBodies()
	if err != nil {
		return nil, err
	}

	agg := &models.Statistics{ByType: make(map[string]map[string]models.StatMap)}
	var mu sync.Mutex
	var succeeded int32

	var g errgroup.Group
	g.SetLimit(gen1QueryConcurrency)

	attempted := 0
	for typeName, body := range bodies {
		attempted++
		typeName, body := typeName, body
		g.Go(func() error {
			respBody, err := c.post(ctx, statisticsPath, body)
			if err != nil {
				log.Warnf("cluster %q: Gen1 stats query for %s failed: %v", c.name, typeName, err)
				return nil // graceful degradation (ADR 0002)
			}
			parsed, err := models.ParseStatistics(respBody)
			if err != nil {
				log.Warnf("cluster %q: failed to parse Gen1 stats for %s: %v", c.name, typeName, err)
				return nil
			}
			mu.Lock()
			agg.Merge(parsed)
			mu.Unlock()
			atomic.AddInt32(&succeeded, 1)
			return nil
		})
	}
	_ = g.Wait()

	if attempted > 0 && atomic.LoadInt32(&succeeded) == 0 {
		return nil, fmt.Errorf("cluster %q: all %d Gen1 stat queries failed", c.name, attempted)
	}
	return agg, nil
}
```

Ensure imports `sync`, `sync/atomic`, `fmt`, `errgroup`, `log` are present (they already are, used by `GetStatisticsV5`).

- [ ] **Step 5: Run the isolation test + full package**

Run: `go test ./internal/powerflex/ -run TestGen1StatsPerTypeIsolation -v`
Expected: PASS.
Run: `go test ./internal/powerflex/ ./internal/models/`
Expected: PASS — existing Gen1 collector tests still emit the same metric set (behavior preserved for unchanged fixtures).

- [ ] **Step 6: Add the all-types-fail test**

Add to `client_test.go`:

```go
func TestGen1StatsAllTypesFail(t *testing.T) {
	g := newMockGateway(t)
	g.failAllStats = true // make every querySelectedStatistics return 500 (add this switch in the handler)
	client := g.clientNamed(t, "c1")
	if _, err := client.GetStatistics(context.Background()); err == nil {
		t.Fatal("expected error when every Gen1 stat query fails")
	}
}
```

Add a `failAllStats bool` to the mock and short-circuit the handler to 500 when set. Run:
`go test ./internal/powerflex/ -run 'TestGen1Stats' -v` → PASS.

- [ ] **Step 7: Full gate + commit**

Run: `make ci`
Expected: PASS.
```bash
git add internal/powerflex/client.go internal/powerflex/client_test.go
git commit -m "feat(powerflex): Gen1 per-type concurrent stats with graceful degradation (ADR 0002)"
```

---

## Slice 2 — New Gen1 stats (WS2-11/12/15/17)

### Task 5: Validate candidate stat names against the spec

**Files:**
- Read-only: `docs/swagger/11231-4.5.5-Manager_v4-8-0.json`

- [ ] **Step 1: Confirm each candidate name is a documented property**

Run, and record which names are present (only present names get added):
```bash
for n in degradedHealthyCapacityInKb fwdRebuildCapacityInKb bckRebuildCapacityInKb \
         normRebuildCapacityInKb rebalanceCapacityInKb snapCapacityInUseInKb \
         pendingMovingInBckRebuildJobs activeMovingInBckRebuildJobs \
         pendingMovingOutBckRebuildJobs activeMovingOutBckRebuildJobs \
         pendingMovingInRebalanceJobs activeMovingInRebalanceJobs \
         pendingMovingInFwdRebuildJobs activeMovingInFwdRebuildJobs \
         targetReadLatency targetWriteLatency journalerReadLatency journalerWriteLatency; do
  printf '%-34s ' "$n"; grep -qo "\"$n\"" docs/swagger/11231-4.5.5-Manager_v4-8-0.json && echo PRESENT || echo absent
done
```
Expected: a mix. **Record the PRESENT set.** Any `absent` name is dropped from this milestone (note it in the commit message as deferred). Also note which *type* the spec attaches each to (search the surrounding schema, e.g. `StoragePoolStatistics`/`ProtectionDomainStatistics`/`SystemStatistics` definitions) so it is added to the right type entry only.

- [ ] **Step 2: No commit** (read-only validation; results feed Task 6).

### Task 6: Add validated stats to querySelectedStatistics.json

**Files:**
- Modify: `internal/powerflex/querySelectedStatistics.json`
- Modify: `internal/powerflex/testdata/statistics.json` (add the new fields so tests exercise them)
- Test: `internal/powerflex/collector_test.go` or `derivations_test.go`

- [ ] **Step 1: Write a failing test asserting the new metrics emit**

Add a test (e.g. in `collector_test.go`) that runs a Gen1 collection against the fixtures and asserts the new metric names appear. Use the existing gather helper; assert a representative subset (only names confirmed PRESENT in Task 5 — adjust the list to the validated set):

```go
func TestGen1NewCoverageMetrics(t *testing.T) {
	names := gatherGen1MetricNames(t) // existing/registry-gather helper; adapt name
	for _, want := range []string{
		"pflex_storagepool_degraded_healthy_capacity_in_kb",
		"pflex_storagepool_bck_rebuild_capacity_in_kb",
		"pflex_protectiondomain_rebalance_capacity_in_kb",
		"pflex_storagepool_snap_capacity_in_use_in_kb",
		"pflex_protectiondomain_latency", // target/journaler latency carries op label
	} {
		if !names[want] {
			t.Errorf("missing expected Gen1 coverage metric %q", want)
		}
	}
}
```

Adapt `gatherGen1MetricNames` to the actual gather pattern (see `collector_test.go`). Only assert names whose source stat was PRESENT in Task 5 AND whose object type you added them to.

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/powerflex/ -run TestGen1NewCoverageMetrics -v`
Expected: FAIL — metrics absent.

- [ ] **Step 3: Add validated stat names to the JSON**

Edit `internal/powerflex/querySelectedStatistics.json`: append each PRESENT stat name to the `properties` array of the correct type entry (System/StoragePool/ProtectionDomain) per Task 5's type mapping. Example (StoragePool entry — add only validated names):

```
        "degradedHealthyCapacityInKb",
        "snapCapacityInUseInKb",
        "bckRebuildCapacityInKb",
        "fwdRebuildCapacityInKb",
        "rebalanceCapacityInKb",
        "targetReadLatency",
        "targetWriteLatency",
        "journalerReadLatency",
        "journalerWriteLatency",
```

Keep the JSON valid: `python3 -c "import json;json.load(open('internal/powerflex/querySelectedStatistics.json'))"` → no error.

- [ ] **Step 4: Add the fields to the test fixture**

Edit `internal/powerflex/testdata/statistics.json`: add the new scalar fields (numbers) and latency accumulators (Bwc-shaped objects) to the relevant type/object so the test exercises them. Example latency value: `"targetReadLatency": {"numOccured": 5, "numSeconds": 1, "totalWeightInKb": 250}`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/powerflex/ -run TestGen1NewCoverageMetrics -v`
Expected: PASS.

- [ ] **Step 6: Full gate + commit**

Run: `make ci`
Expected: PASS.
```bash
git add internal/powerflex/querySelectedStatistics.json internal/powerflex/testdata/statistics.json internal/powerflex/collector_test.go
git commit -m "feat(powerflex): add WS2-11/12/15/17 Gen1 capacity/rebuild/latency stats"
```

---

## Slice 3 — State metrics (WS2-09 Sdt, WS2-20 device wear/temp)

### Task 7: Add model fields (spec-validated)

**Files:**
- Modify: `internal/models/instances.go`
- Test: `internal/models/instances_test.go`

- [ ] **Step 1: Confirm field names in the spec**

Run (Sdt in 5.0.0, device fields likely in both):
```bash
for n in sdtState temperatureState ssdEndOfLifeState errorState maintenanceState; do
  printf '%-20s ' "$n"; grep -qo "\"$n\"" docs/swagger/11231-5.0.0.json && echo PRESENT-5.0 || echo absent-5.0
done
```
Record PRESENT names; use the spec's exact spelling for json tags.

- [ ] **Step 2: Write the failing test**

Add to `internal/models/instances_test.go` a fixture with the new fields and assert they parse:

```go
func TestInstanceStateFields(t *testing.T) {
	in, err := ParseInstances([]byte(`{
		"sdtList":[{"id":"sdt1","name":"t1","sdtState":"Normal","links":[]}],
		"deviceList":[{"id":"dev1","name":"d1","deviceState":"Normal","temperatureState":"NormalTemperature","ssdEndOfLifeState":"NormalEndOfLife","errorState":"None","links":[]}]
	}`))
	if err != nil { t.Fatal(err) }
	if got := in.Get(TypeSdt)[0].SdtState; got != "Normal" {
		t.Errorf("SdtState = %q", got)
	}
	d := in.Get(TypeDevice)[0]
	if d.TemperatureState != "NormalTemperature" || d.SsdEndOfLifeState != "NormalEndOfLife" || d.ErrorState != "None" {
		t.Errorf("device wear fields = %q/%q/%q", d.TemperatureState, d.SsdEndOfLifeState, d.ErrorState)
	}
}
```

- [ ] **Step 3: Run it to verify it fails**

Run: `go test ./internal/models/ -run TestInstanceStateFields -v`
Expected: FAIL — fields undefined.

- [ ] **Step 4: Add the fields**

In `internal/models/instances.go`, near the existing state fields (after `DeviceState` at line ~53), add (use spec spellings confirmed in Step 1):

```go
	// Device wear/health inputs (WS2-20).
	TemperatureState  string `json:"temperatureState,omitempty"`
	SsdEndOfLifeState string `json:"ssdEndOfLifeState,omitempty"`
	ErrorState        string `json:"errorState,omitempty"`
	// Sdt operational state (Gen2 NVMe/TCP target, WS2-09).
	SdtState string `json:"sdtState,omitempty"`
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/models/ -run TestInstanceStateFields -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/models/instances.go internal/models/instances_test.go
git commit -m "feat(models): add Sdt and device wear/temp/error state fields (WS2-09/20)"
```

### Task 8: Emit Sdt health + device wear into state samples

**Files:**
- Modify: `internal/powerflex/state.go`
- Modify: `internal/powerflex/testdata/instances-gen2.json` (Sdt with state), `testdata/instances.json` (device wear fields)
- Test: `internal/powerflex/state_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/powerflex/state_test.go` (adapt helper names to existing tests):

```go
func TestSdtAndDeviceWearState(t *testing.T) {
	// Gen2 cluster with an Sdt whose state is degraded, and a device with a failed wear state.
	samples := deriveStateSamplesForFixture(t, "instances-gen2.json", GenerationGen2)
	if !hasSample(samples, "pflex_sdt_health") {
		t.Error("missing pflex_sdt_health for Gen2")
	}
	if !hasSample(samples, "pflex_sdt_info") {
		t.Error("missing pflex_sdt_info for Gen2")
	}
	// Gen1 Sdt must NOT appear (Sdt is Gen2-only).
	g1 := deriveStateSamplesForFixture(t, "instances.json", GenerationGen1)
	if hasSample(g1, "pflex_sdt_health") {
		t.Error("pflex_sdt_health must not be emitted for Gen1")
	}
}
```

Adapt `deriveStateSamplesForFixture`/`hasSample` to the existing test helpers (see `state_test.go`).

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/powerflex/ -run TestSdtAndDeviceWearState -v`
Expected: FAIL.

- [ ] **Step 3: Implement state.go changes**

In `internal/powerflex/state.go`:

Add the Sdt label fn:
```go
func sdtStateLabels(o *models.Instance) []Label {
	return []Label{{"sdt_state", o.SdtState}}
}
```

Extend device labels (WS2-20) — keep existing `device_state` first:
```go
func deviceStateLabels(o *models.Instance) []Label {
	return []Label{
		{"device_state", o.DeviceState},
		{"temperature_state", o.TemperatureState},
		{"ssd_end_of_life_state", o.SsdEndOfLifeState},
		{"error_state", o.ErrorState},
	}
}
```

In `deriveStateSamples`, after the Sdc line, add the Gen2-only Sdt emission:
```go
	if gen == GenerationGen2 {
		samples = append(samples, emitState(clusterName, systemID, in, rel, builders, models.TypeSdt, metricPrefix[models.TypeSdt], sdtStateLabels)...)
	}
```

Add severity entries to `healthSeverity` for the new state strings (confirm actual strings from the spec; typical PowerFlex values):
```go
	// Device wear/temperature/error (WS2-20)
	"NormalTemperature":   0,
	"NormalEndOfLife":     0,
	"None":                0,
	"AboveThreshold":      1,
	"EndOfLifeWarning":    1,
	"Error":               2,
	"EndOfLife":           2,
```

(If any string already maps, do not duplicate the key — `gofmt`/compile will not catch duplicate map keys, but `go vet` will; resolve by keeping one.)

- [ ] **Step 4: Add fixture state values**

Edit `testdata/instances-gen2.json`: ensure an `sdtList` entry with `"sdtState"` set (e.g. `"Normal"`). Edit `testdata/instances.json`: add `temperatureState`/`ssdEndOfLifeState`/`errorState` to a device.

- [ ] **Step 5: Run the test + label-consistency guard**

Run: `go test ./internal/powerflex/ -run 'TestSdtAndDeviceWearState|TestMixedGenerationMetricsValid' -v`
Expected: PASS (device `_info` label set changed consistently for both gens; `_health` unaffected by extra labels since it's a scalar).

- [ ] **Step 6: Full gate + commit**

Run: `make ci`
Expected: PASS.
```bash
git add internal/powerflex/state.go internal/powerflex/testdata/instances.json internal/powerflex/testdata/instances-gen2.json internal/powerflex/state_test.go
git commit -m "feat(powerflex): emit pflex_sdt_health (Gen2) + device wear/temp state (WS2-09/20)"
```

---

## Slice 4 — Inventory counts + docs + dashboards

### Task 9: Inventory-count derivation (WS2-13)

**Files:**
- Modify: `internal/models/instances.go` (add `numOf*` fields)
- Create: `internal/powerflex/inventory.go`
- Modify: `internal/powerflex/collector.go` (call it in `buildSamplesGen1` and `buildSamplesGen2`)
- Test: `internal/powerflex/inventory_test.go`

- [ ] **Step 1: Confirm field names + write failing test**

Confirm in spec: `grep -o '"numOf[A-Za-z]*"' docs/swagger/11231-4.5.5-Manager_v4-8-0.json | sort -u | head`.
Add fields to `models.Instance` (only confirmed ones), e.g.:
```go
	// Inventory counts (WS2-13), present on System/ProtectionDomain.
	NumOfVolumes int `json:"numOfVolumes,omitempty"`
	NumOfSds     int `json:"numOfSds,omitempty"`
	NumOfDevices int `json:"numOfDevices,omitempty"`
```
Write `internal/powerflex/inventory_test.go`:
```go
package powerflex

import "testing"

func TestInventorySamples(t *testing.T) {
	samples := inventorySamplesForFixture(t, "instances.json", GenerationGen1) // adapt helper
	if !hasSample(samples, "pflex_cluster_num_of_volumes") {
		t.Error("missing pflex_cluster_num_of_volumes")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/powerflex/ -run TestInventorySamples -v`
Expected: FAIL — `inventorySamples` undefined.

- [ ] **Step 3: Implement inventory.go**

Create `internal/powerflex/inventory.go`:
```go
package powerflex

import "github.com/fjacquet/pflex_exporter/internal/models"

// inventorySamples emits scalar count gauges (pflex_<obj>_num_of_*) from instance
// properties (authoritative API counts, not len() of the relations graph). WS2-13.
func inventorySamples(clusterName, systemID string, in *models.Instances, rel *models.Relations, gen string) []Sample {
	builders := labelBuildersGen1
	if gen == GenerationGen2 {
		builders = labelBuildersGen2
	}
	var samples []Sample
	// System-scoped counts.
	for _, sys := range in.Get(models.TypeSystem) {
		base, ok := builders[models.TypeSystem](clusterName, systemID, sys, in, rel)
		if !ok {
			continue
		}
		samples = append(samples,
			Sample{Name: metricPrefix[models.TypeSystem] + "_num_of_volumes", Labels: base, Value: float64(sys.NumOfVolumes)},
			Sample{Name: metricPrefix[models.TypeSystem] + "_num_of_sds", Labels: base, Value: float64(sys.NumOfSds)},
			Sample{Name: metricPrefix[models.TypeSystem] + "_num_of_devices", Labels: base, Value: float64(sys.NumOfDevices)},
		)
	}
	return samples
}
```
(If `metricPrefix[TypeSystem]` is `pflex_cluster`, the names become `pflex_cluster_num_of_*` — confirm via `grep TypeSystem internal/powerflex/metrics.go`.)

- [ ] **Step 4: Wire into the collector**

In `internal/powerflex/collector.go`, within both `buildSamplesGen1` and `buildSamplesGen2`, append:
```go
	samples = append(samples, inventorySamples(cs.Name, systemID, instances, relations, <gen>)...)
```
Use `GenerationGen1`/`GenerationGen2` respectively. Match the exact local variable names already in scope in each function (check the surrounding code).

- [ ] **Step 5: Add fixture counts + run test**

Add `numOfVolumes`/`numOfSds`/`numOfDevices` to the System object in `testdata/instances.json`.
Run: `go test ./internal/powerflex/ -run TestInventorySamples -v` → PASS.

- [ ] **Step 6: Full gate + commit**

Run: `make ci`
Expected: PASS.
```bash
git add internal/models/instances.go internal/powerflex/inventory.go internal/powerflex/collector.go internal/powerflex/inventory_test.go internal/powerflex/testdata/instances.json
git commit -m "feat(powerflex): emit cluster inventory counts from instance properties (WS2-13)"
```

### Task 10: Documentation

**Files:**
- Modify: `docs/metrics.md`, `docs/dashboards.md`, `CLAUDE.md` (only if a constraint changed)

- [ ] **Step 1: Document every new + newly-surfaced metric**

In `docs/metrics.md`, add entries for: the WS2-11/15 metrics that were already collected but undocumented (`pflex_*_degraded_failed_capacity_in_kb`, `_failed_capacity_in_kb`, `_spare_capacity_in_kb`, `_net_snapshot_capacity_in_kb`), the new Slice-2 stats, `pflex_sdt_health`/`_info`, the device wear/temp/error labels on `pflex_device_info`, and `pflex_cluster_num_of_*`. Group under the existing object-type sections; keep the file's existing style.

- [ ] **Step 2: Note the Gen1 collection change**

In `CLAUDE.md`, update the Gen1 description (line ~31) to note that `querySelectedStatistics` is now issued **per type concurrently** (ADR 0002), so Gen1 and Gen2 share the per-type-fan-out shape. One sentence; reference ADR 0002.

- [ ] **Step 3: Commit**

```bash
git add docs/metrics.md docs/dashboards.md CLAUDE.md
git commit -m "docs: document WS2 Milestone A metrics and Gen1 per-type collection"
```

### Task 11: Dashboards

**Files:**
- Modify: `grafana/gen1/08-cluster-capacity.json`, `grafana/gen1/03-storage-pools.json`, `grafana/gen1/07-protection-domains.json`, `grafana/gen1/02-devices.json`, `grafana/gen2/06-storage-node.json` (+ a new Sdt panel target)

- [ ] **Step 1: Add panels for the new metrics**

Add panels (follow the existing panel JSON shape in each file): degraded/failed/spare capacity and snapshot capacity on the capacity/pool/PD dashboards; rebuild/rebalance remaining-capacity and job-count panels on the PD/pool dashboards; target/journaler latency on the PD dashboard; device wear/temperature state table on the devices dashboard; `pflex_sdt_health` on a Gen2 dashboard (e.g. add an Sdt health row to `grafana/gen2/06-storage-node.json` or a new panel). Use the PromQL conventions in `docs/dashboards.md` (gauges; `sum`/`avg by`, never `rate()`).

- [ ] **Step 2: Validate each edited dashboard is valid JSON**

Run: `for f in grafana/gen1/*.json grafana/gen2/*.json; do python3 -c "import json,sys;json.load(open('$f'))" || echo "BAD: $f"; done`
Expected: no `BAD:` lines.

- [ ] **Step 3: Cross-check panels reference only emitted metrics**

Run the audit cross-check from the previous milestone:
```bash
python3 scripts/audit/extract_dashboards.py grafana | sort -u > /tmp/dash_metrics.txt
PFLEX_DUMP_METRICS=1 go test ./internal/powerflex/ -run TestDumpEmittedMetricNames -v 2>&1 | grep -oE 'pflex_[a-z0-9_]+' | sort -u > /tmp/emitted_names.txt
comm -23 /tmp/dash_metrics.txt /tmp/emitted_names.txt
```
Expected: empty (every new panel references an emitted metric). Any leftover is either a fixture-coverage gap (add the field to fixtures) or a typo (fix the panel).

- [ ] **Step 4: Commit**

```bash
git add grafana/
git commit -m "feat(grafana): panels for WS2 Milestone A capacity/rebuild/health metrics"
```

### Task 12: Final verification

- [ ] **Step 1: Full gate**

Run: `make ci`
Expected: PASS (gofmt, vet, golangci-lint, `go test -race`, govulncheck).

- [ ] **Step 2: Docs site builds**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict 2>&1 | tail -5`
Expected: builds clean (ADR 0002/0003 may be INFO not-in-nav, which is acceptable as before).

- [ ] **Step 3: Confirm dump-test set grew**

Run: `PFLEX_DUMP_METRICS=1 go test ./internal/powerflex/ -run TestDumpEmittedMetricNames -v 2>&1 | grep -c '^pflex_'`
Expected: a count higher than the pre-milestone 81 (new metrics now emit).

- [ ] **Step 4: Report** the new metric count, which WS2 IDs landed, and any stat names deferred (absent from spec in Task 5/7) for Milestone B.

---

## Self-Review (completed by plan author)

**Spec coverage:** Component 1 (isolation) → Tasks 2–4; ADRs → Task 1; WS2-11/12/15/17 → Tasks 5–6; WS2-09/20 → Tasks 7–8; WS2-13 → Task 9; docs → Task 10; dashboards → Task 11; testing (fault injection, OTLP+Prom, label consistency, spec validation) → Tasks 4/6/8/9/11; phasing matches the spec's four slices. All spec sections mapped.

**Placeholder scan:** No "TBD"/"add error handling". Spec-name validation (Tasks 5, 7, 9 Step 1) is a deliberate runtime check against the pinned spec, not a placeholder — the plan adds only confirmed names and defers the rest, which is correct because guessing a name risks the very batch-failure this milestone prevents (now isolated per-type anyway). Mock-helper names (`newMockGateway`, `clientNamed`, `readFixture`, `gatherGen1MetricNames`, `hasSample`, `deriveStateSamplesForFixture`) are flagged "adapt to existing" because each task's first step greps the existing tests for the real names.

**Type consistency:** `gen1PerTypeBodies()` (Task 2) → used in `GetStatistics` (Task 4). `Statistics.Merge` (Task 3) → used in Task 4. `gen1QueryConcurrency` defined Task 4. New `models.Instance` fields (Tasks 7, 9) → consumed in Tasks 8, 9. `inventorySamples` (Task 9) signature consistent between definition and collector wiring. Metric names referenced in dashboard cross-check (Task 11) match those asserted in Tasks 6/8/9.
