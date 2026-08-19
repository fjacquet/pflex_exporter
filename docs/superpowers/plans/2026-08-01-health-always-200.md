# /health always-200 (JSON) + /livez /readyz (pflex_exporter) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `/livez`/`/readyz` (always-200, no state). Convert `/health`
from plain text to a JSON body matching the rest of the family, always 200.

**Architecture:** New `staticOKHandler` registered at `/livez`/`/readyz`.
`healthHandler` (`main.go:379-396`, a method on `*Server`) rewritten to
build a JSON body from `s.store.Load().PerCluster` instead of writing plain
text and a 503.

**Tech Stack:** Go, `net/http`, `net/http/httptest`.

## Global Constraints

- Repo: `/Users/fjacquet/Projects/pflex_exporter`.
- Spec: `/Users/fjacquet/Projects/obs_exporter/docs/superpowers/specs/2026-08-01-family-health-endpoint-design.md` (bucket C).
- **Breaking change**: `/health`'s body format changes from plain text to JSON, and the status code is always 200 (was 503 when every cluster was down). Call this out explicitly in CHANGELOG `### Changed`.
- `/livez`/`/readyz` are net-new — `### Added` in CHANGELOG.
- ADRs live in `docs/adr/`, no index/README file exists — the only index is `mkdocs.yml`'s nav (`Architecture Decisions:` section, currently only lists ADR-0001 even though 0002/0003 exist — a pre-existing gap, not in scope to backfill here; just add this task's own entry). Next ADR: 0004.
- No `main_test.go` exists in this repo's root package today — this plan creates it.
- `powerflex.ClusterSnapshot.ScrapeError` is a `string`, not an `error` — no `.Error()` call needed.

---

### Task 1: `/livez` `/readyz` + rewrite `/health` to JSON, always 200

**Files:**
- Modify: `main.go:117` (add two `mux.HandleFunc` lines after the existing `/health` registration)
- Modify: `main.go:379-396` (method `(s *Server) healthHandler`, full rewrite)
- Create: `main.go` — add `staticOKHandler` function after `healthHandler`'s closing brace
- Create: `main_test.go`

**Interfaces:**
- Consumes: `powerflex.SnapshotStore` (`internal/powerflex/snapshot.go:56-77`) — `Load() *Snapshot`, `NewSnapshotStore() *SnapshotStore` (seeds an empty, non-nil snapshot — `Load()` never returns nil). `powerflex.Snapshot` (`internal/powerflex/snapshot.go:19-24`): `PerCluster map[string]*ClusterSnapshot` (plus unexported `byName`/`names`, irrelevant here). `powerflex.ClusterSnapshot` (`internal/powerflex/snapshot.go:9-16`): `Cluster string`, `Up bool`, `Generation string`, `ScrapeError string`, `LastScrape time.Time`, `Samples []Sample`.
- Produces: `staticOKHandler(w http.ResponseWriter, _ *http.Request)`. `(s *Server) healthHandler(w http.ResponseWriter, _ *http.Request)` — signature unchanged, body behavior changes.

- [ ] **Step 1: Write failing tests**

Create `main_test.go`:

```go
package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fjacquet/pflex_exporter/internal/powerflex"
)

func TestLivezReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/livez", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadyzReturnsOK(t *testing.T) {
	rec := httptest.NewRecorder()
	staticOKHandler(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestHealthReturns200WhenAllClustersDown(t *testing.T) {
	store := powerflex.NewSnapshotStore()
	store.Store(powerflex.BuildSnapshot([]*powerflex.ClusterSnapshot{
		{Cluster: "pflex-01", Up: false, ScrapeError: "login POST: status 401", LastScrape: time.Now()},
	}))
	server := &Server{store: store}

	rec := httptest.NewRecorder()
	server.healthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body struct {
		Clusters []struct {
			Cluster string `json:"cluster"`
			OK      bool   `json:"ok"`
			Err     string `json:"err"`
		} `json:"clusters"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(body.Clusters) != 1 || body.Clusters[0].OK {
		t.Fatalf("clusters = %+v, want one cluster with ok=false", body.Clusters)
	}
	if body.Clusters[0].Err == "" {
		t.Fatalf("err field empty, want the scrape failure message")
	}
}

func TestHealthReturns200BeforeFirstCycle(t *testing.T) {
	server := &Server{store: powerflex.NewSnapshotStore()}

	rec := httptest.NewRecorder()
	server.healthHandler(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
```

Check `main.go`'s own import path for `internal/powerflex` (e.g.
`"github.com/fjacquet/pflex_exporter/internal/powerflex"`) and use the same
string. `Server` (`main.go:56-...`) must be constructible with just a
`store` field set for this test — if `Server`'s zero value plus `store` set
doesn't satisfy `healthHandler`'s dependencies (it shouldn't need anything
else), no changes needed there; if it does need other fields, use
`&Server{store: store}` and add only the minimum additional field the
compiler demands.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -run 'TestLivezReturnsOK|TestReadyzReturnsOK|TestHealthReturns200' -v`
Expected: `TestLivezReturnsOK`/`TestReadyzReturnsOK` FAIL with `undefined: staticOKHandler`. `TestHealthReturns200*` FAIL to decode JSON (old handler writes plain text).

- [ ] **Step 3: Add `staticOKHandler` and register `/livez` `/readyz`**

In `main.go`, change line 117 from:

```go
	mux.HandleFunc("/health", s.healthHandler)
```

to:

```go
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/livez", staticOKHandler)
	mux.HandleFunc("/readyz", staticOKHandler)
```

After `healthHandler`'s closing brace (currently line 396), add:

```go

// staticOKHandler always answers 200 — no cluster state, no collection
// state, nothing that can make it fail. /livez and /readyz both use it: a
// probe wired here can never be the reason a healthy process gets restarted
// or pulled from rotation.
func staticOKHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
```

- [ ] **Step 4: Rewrite `healthHandler`**

Replace the full function (`main.go:379-396`, including its doc comment) with:

```go
// healthHandler always answers 200. The JSON body reports every configured
// cluster's cached status from the last collection cycle.
func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	type clusterHealth struct {
		Cluster    string `json:"cluster"`
		OK         bool   `json:"ok"`
		LastScrape string `json:"last_scrape"`
		Err        string `json:"err,omitempty"`
	}
	snap := s.store.Load()
	out := struct {
		Clusters []clusterHealth `json:"clusters"`
	}{}
	for _, cs := range snap.PerCluster {
		out.Clusters = append(out.Clusters, clusterHealth{
			Cluster:    cs.Cluster,
			OK:         cs.Up,
			LastScrape: cs.LastScrape.Format(time.RFC3339),
			Err:        cs.ScrapeError,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}
```

Add `"encoding/json"` to `main.go`'s import block if not already present
(check with `go build ./...` in Step 6 — `fmt` may become unused if
`healthHandler` was its only caller in this file, remove it if so).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test . -run 'TestLivezReturnsOK|TestReadyzReturnsOK|TestHealthReturns200' -v`
Expected: all PASS.

- [ ] **Step 6: Run full test suite and build**

Run: `go build ./... && go test ./...`
Expected: builds clean (fix any now-unused imports), all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add main.go main_test.go
git commit -m "feat: /health returns JSON, always 200; add /livez /readyz

/health previously returned plain text and 503 when every cluster was
down. It now emits JSON (clusters: [{cluster, ok, last_scrape, err}])
and always answers 200 — a cluster being unreachable is data the
exporter reports, not a failure of the exporter itself. Matches
obs_exporter's ADR-0013/ADR-0014 pattern.

BREAKING CHANGE: /health's body format changes from plain text to
JSON; its status code is always 200 (previously 503 when every
cluster was unreachable)."
```

---

### Task 2: Chart, ADR, docs, CHANGELOG

**Files:**
- Modify: `charts/pflex-exporter/values.yaml:50-57`
- Create: `docs/adr/0004-health-always-200-and-static-probes.md`
- Modify: `mkdocs.yml:58-59` (add nav entry under `Architecture Decisions:`)
- Modify: `CHANGELOG.md` (under existing `## [Unreleased]`)
- Modify: any deployment/monitoring docs mentioning `/health`'s body format or probe wiring — grep first (see Step 1)

**Interfaces:**
- Consumes: nothing (docs-only task).
- Produces: nothing.

- [ ] **Step 1: Find every doc mentioning `/health` as a probe target or describing its body/status**

Run: `grep -rln '/health\|livenessProbe\|readinessProbe' docs/ README.md 2>/dev/null`

Update every hit: probes now use `/livez`/`/readyz` (always 200, no cluster
state); `/health` is JSON now (`clusters: [{cluster, ok, last_scrape, err}]`),
always 200 — not plain text, not ever 503.

- [ ] **Step 2: Update the chart**

In `charts/pflex-exporter/values.yaml:50-57`, change:

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
readinessProbe:
  httpGet:
    path: /health
    port: http
```

to:

```yaml
livenessProbe:
  httpGet:
    path: /livez
    port: http
readinessProbe:
  httpGet:
    path: /readyz
    port: http
```

- [ ] **Step 3: Write ADR-0004**

Create `docs/adr/0004-health-always-200-and-static-probes.md`:

```markdown
# `/livez` `/readyz`, and `/health` always answering 200

## Status

Accepted (2026-08-01)

## Context

Same argument as obs_exporter's ADR-0013 and ADR-0014, applied here in one
pass: an exporter is a probe. "Cluster unreachable" is data it reports, not
a failure of the exporter process. Coupling that fact to an HTTP status code
on any endpoint — the chart's `livenessProbe`/`readinessProbe`, or the
informational `/health` — risks something downstream (kubelet, a dashboard,
a script) treating a healthy, correctly-reporting exporter as down.

`charts/pflex-exporter/values.yaml` wired both `livenessProbe` and
`readinessProbe` to `/health`, which answered 503 when every configured
cluster was unreachable. As a *liveness* check this was always wrong: no
restart makes an unreachable cluster reachable.

## Decision

Two new endpoints, `/livez` and `/readyz`, both `staticOKHandler` — always
`200 OK`, no `SnapshotStore` read, nothing that can make either fail once
the process is running. The chart's default probes now point at them.

`/health`'s `healthHandler` no longer writes plain text or a 503. It always
answers 200 with a JSON body: `clusters: [{cluster, ok, last_scrape, err}]`,
built from the same `SnapshotStore` `/metrics` reads.

## Consequences

- **Breaking**: `/health`'s response body changes from plain text
  (`"OK"`/`"OK (starting)"`/`"UNHEALTHY: ..."`) to JSON, and its status code
  is always 200 (previously 503 when every cluster was down). Anything
  parsing the old text format or gating on the old status code needs
  updating.
- Chart default probe wiring changes; a fresh `helm install` or an upgrade
  without pinned probe overrides gets the fix automatically.
- Alert on a per-cluster `_up` metric (or `/health`'s body), never on any
  probe's HTTP status.
```

- [ ] **Step 4: Add the ADR to `mkdocs.yml`'s nav**

In `mkdocs.yml`, after line 59 (the ADR 0001 nav entry), add:

```yaml
      - ADR 0004 — Health always 200, static probes: adr/0004-health-always-200-and-static-probes.md
```

(Leave the existing gap where 0002/0003 aren't in the nav alone — out of
scope for this change.)

- [ ] **Step 5: CHANGELOG entry**

In `CHANGELOG.md`, under the existing `## [Unreleased]` heading, add:

```markdown
### Added

- `/livez` and `/readyz`: probe endpoints that always answer 200, with no
  dependency on cluster reachability or the collection cycle. See ADR-0004.

### Changed

- `/health` always answers 200, never 503, and its body is now JSON
  (`clusters: [{cluster, ok, last_scrape, err}]`) instead of plain text.
  See ADR-0004. **Breaking**: anything parsing the old plain-text body or
  gating on the old 503 status needs updating.
- The chart's default `livenessProbe`/`readinessProbe` now point at
  `/livez`/`/readyz` instead of `/health`.
```

- [ ] **Step 6: Lint chart + build docs**

Run: `helm lint charts/pflex-exporter` (or the exact CI invocation from `.github/workflows/` if different)
Expected: exits 0.

Run: `mkdocs build --strict` (if `mkdocs.yml` present)
Expected: exits 0.

- [ ] **Step 7: Commit**

```bash
git add charts/pflex-exporter/values.yaml docs/adr/0004-health-always-200-and-static-probes.md \
  mkdocs.yml CHANGELOG.md
git commit -m "docs+chart: record ADR-0004, repoint chart probes to /livez /readyz"
```
