# Swagger Validation Audit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit the pflex_exporter against the in-scope Dell PowerFlex swagger contracts (auth, Gen1 Block API, Gen2 Block Storage API) and the 16 Grafana dashboards, then fix the findings under a reality-outranks-spec trust rule.

**Architecture:** Hybrid audit — throwaway extraction scripts produce precise set-diffs (swagger paths/fields, emitted metrics, dashboard metrics); semantic contracts (auth flow, Gen1 `querySelectedStatistics` body, Gen2 dtapi payload) are read by hand. Findings land in one severity-ranked report; fixes apply in risk tiers, each gated by `make ci` + the semgrep write-hook.

**Tech Stack:** Go 1.x (`internal/powerflex`), Prometheus client_golang registry gather, Python 3 (stdlib `json`) for JSON extraction, Grafana dashboard JSON, MkDocs.

**Working branch:** `audit/swagger-validation` (already created). The design spec lives at `docs/superpowers/specs/2026-06-14-swagger-validation-audit.md`.

**Trust hierarchy (apply to EVERY discrepancy):** `passing tests + documented live behavior + CLAUDE.md notes  >  swagger spec`. Code that disagrees with the spec but agrees with reality → "spec-is-wrong" note, **no code change**. Code that disagrees with both → real bug, fix it. The dtapi `metrics` field MUST stay a JSON array — never "fix" it to the spec's string form (reintroduces the v0.6.2 HTTP 500 regression).

---

## File Structure

**Created (audit artifacts):**
- `docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md` — the severity-ranked findings report (deliverable).
- `scripts/audit/extract_swagger.py` — extract paths + response field names from in-scope swagger files.
- `scripts/audit/extract_dashboards.py` — extract distinct `pflex_*` metric names referenced by `grafana/**`.
- `internal/powerflex/dump_metrics_test.go` — temporary test that gathers the emitted metric-name set from the mock-gateway fixtures (Gen1 + Gen2). Removed (or kept guarded) at the end.

**Modified (fixes, only as findings dictate):**
- `grafana/gen1/*.json`, `grafana/gen2/*.json` — dead-panel fixes (Tier 1).
- `docs/metrics.md`, `docs/dashboards.md` — doc mismatches (Tier 1).
- `internal/powerflex/*.go` — real contract bugs (Tier 2, only where reality+spec agree and code is wrong).

**Pinned reference (Task 1):** all 11 files under `docs/swagger/` committed as-is.

---

## Task 1: Pin the swagger reference

**Files:**
- Modify: git index (add `docs/swagger/`)

- [ ] **Step 1: Confirm the 11 files are present and untracked**

Run: `git status --short docs/swagger/ && ls docs/swagger/ | wc -l`
Expected: 11 `??` entries (or already staged), count `11`.

- [ ] **Step 2: Stage and commit the pinned specs**

```bash
git add docs/swagger/
git commit -m "docs(swagger): pin Dell PowerFlex REST API specs as audit reference"
```

- [ ] **Step 3: Verify clean tree**

Run: `git status --short docs/swagger/`
Expected: empty output.

---

## Task 2: Swagger extraction script

**Files:**
- Create: `scripts/audit/extract_swagger.py`

- [ ] **Step 1: Write the extraction script**

```python
#!/usr/bin/env python3
"""Extract paths and response schema field names from in-scope PowerFlex swagger specs.

Usage: python3 scripts/audit/extract_swagger.py docs/swagger/<file>.json
Prints: every path+method, and the flattened set of response property names per 2xx schema.
"""
import json
import sys


def schema_fields(schema, defs, seen=None):
    """Recursively collect property names from a (possibly $ref) schema."""
    if seen is None:
        seen = set()
    fields = set()
    if not isinstance(schema, dict):
        return fields
    ref = schema.get("$ref")
    if ref:
        name = ref.split("/")[-1]
        if name in seen:
            return fields
        seen.add(name)
        return schema_fields(defs.get(name, {}), defs, seen)
    for key, val in schema.get("properties", {}).items():
        fields.add(key)
        fields |= schema_fields(val, defs, seen)
    if "items" in schema:
        fields |= schema_fields(schema["items"], defs, seen)
    return fields


def main(path):
    spec = json.load(open(path))
    defs = spec.get("definitions", {}) or spec.get("components", {}).get("schemas", {})
    print(f"# {path} :: {spec.get('info', {}).get('title', '?')}")
    for p, methods in sorted(spec.get("paths", {}).items()):
        for method, op in methods.items():
            if method not in ("get", "post", "put", "delete", "patch"):
                continue
            print(f"\n{method.upper()} {p}")
            params = [pr.get("name") for pr in op.get("parameters", []) if pr.get("name")]
            if params:
                print(f"  params: {sorted(params)}")
            resp = op.get("responses", {})
            for code in ("200", "201"):
                sch = resp.get(code, {}).get("schema") or \
                    resp.get(code, {}).get("content", {}).get("application/json", {}).get("schema")
                if sch:
                    fields = sorted(schema_fields(sch, defs))
                    print(f"  {code} fields: {fields}")


if __name__ == "__main__":
    main(sys.argv[1])
```

- [ ] **Step 2: Run it against the auth spec and verify output**

Run: `python3 scripts/audit/extract_swagger.py docs/swagger/11227-5.0.0.json`
Expected: lists `POST /rest/auth/login`, `POST /rest/auth/update-token`, `POST /rest/auth/logout` with response fields. If output is empty, the spec uses a schema layout the script doesn't handle — inspect with `python3 -c "import json;print(list(json.load(open('docs/swagger/11227-5.0.0.json'))['paths'].values())[0])"` and adjust `schema_fields`.

- [ ] **Step 3: Commit**

```bash
git add scripts/audit/extract_swagger.py
git commit -m "chore(audit): add swagger path/field extraction script"
```

---

## Task 3: Dashboard metric extraction script

**Files:**
- Create: `scripts/audit/extract_dashboards.py`

- [ ] **Step 1: Write the extraction script**

```python
#!/usr/bin/env python3
"""Extract distinct pflex_* metric names referenced across grafana/** dashboards.

Usage: python3 scripts/audit/extract_dashboards.py grafana
Prints one metric name per line, sorted, plus a per-file breakdown to stderr.
"""
import json
import os
import re
import sys

METRIC_RE = re.compile(r"pflex_[a-z0-9_]+")


def main(root):
    all_metrics = set()
    for dirpath, _, files in os.walk(root):
        for fn in files:
            if not fn.endswith(".json"):
                continue
            full = os.path.join(dirpath, fn)
            blob = open(full).read()
            found = set(METRIC_RE.findall(blob))
            if found:
                print(f"{full}: {len(found)} metrics", file=sys.stderr)
            all_metrics |= found
    for m in sorted(all_metrics):
        print(m)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else "grafana")
```

- [ ] **Step 2: Run it and capture the referenced set**

Run: `python3 scripts/audit/extract_dashboards.py grafana > /tmp/dash_metrics.txt; wc -l /tmp/dash_metrics.txt`
Expected: ~57 lines (matches the earlier count). Note: this captures metric *names*; label selectors like `{op="total"}` are stripped — name-level matching is sufficient for dead-panel detection.

- [ ] **Step 3: Commit**

```bash
git add scripts/audit/extract_dashboards.py
git commit -m "chore(audit): add dashboard metric-name extraction script"
```

---

## Task 4: Generate the canonical emitted-metric set

**Files:**
- Create: `internal/powerflex/dump_metrics_test.go`
- Reference: `internal/powerflex/collector_test.go`, `internal/powerflex/client_test.go` (existing mock gateway + registry-gather pattern)

- [ ] **Step 1: Read the existing gather pattern**

Run: `grep -n "Gather\|httptest\|newMock\|registry\|prometheus.NewRegistry\|testdata" internal/powerflex/collector_test.go internal/powerflex/client_test.go`
Expected: find how tests spin the mock gateway and gather from a registry. Mirror that exact setup in the new test (helper names, fixture files `instances*.json`, `statistics.json`, `statistics-v5.json`).

- [ ] **Step 2: Write the dump test**

Mirror the existing collector-test harness. The skeleton below assumes the same helpers used in `collector_test.go`; adapt names to whatever Step 1 reveals (e.g. the mock-server constructor and the function that runs one collection cycle and returns a `*PromCollector` or registers it). The goal: register `PromCollector`, gather, print sorted metric family names.

```go
package powerflex

import (
	"fmt"
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

// TestDumpEmittedMetricNames prints the canonical set of emitted metric names
// for the audit. Run explicitly:
//   go test ./internal/powerflex/ -run TestDumpEmittedMetricNames -v
// Adapt the harness calls to match collector_test.go (mock gateway + one cycle).
func TestDumpEmittedMetricNames(t *testing.T) {
	for _, gen := range []string{"gen1", "gen2"} {
		reg := prometheus.NewRegistry()
		// TODO(adapt): build the mock gateway + collector exactly as collector_test.go
		// does for this generation's fixtures, run ONE collection cycle so the
		// SnapshotStore is populated, then register the PromCollector:
		//   pc := NewPromCollector(store)
		//   reg.MustRegister(pc)
		mfs, err := reg.Gather()
		if err != nil {
			t.Fatalf("%s gather: %v", gen, err)
		}
		names := make([]string, 0, len(mfs))
		for _, mf := range mfs {
			names = append(names, mf.GetName())
		}
		sort.Strings(names)
		fmt.Printf("=== EMITTED %s (%d) ===\n", gen, len(names))
		for _, n := range names {
			fmt.Println(n)
		}
	}
}
```

- [ ] **Step 3: Run it and capture the emitted set**

Run: `go test ./internal/powerflex/ -run TestDumpEmittedMetricNames -v 2>&1 | grep -E '^(===|pflex_)' | tee /tmp/emitted_metrics.txt`
Expected: two `=== EMITTED genN ===` blocks listing concrete `pflex_*` names. If a block is empty, the cycle didn't populate the snapshot — re-check the harness adaptation against `collector_test.go`.

- [ ] **Step 4: Verify completeness against the registry, not docs**

Run: `grep -c pflex_ /tmp/emitted_metrics.txt`
Expected: a count comfortably ≥ the 25 in `docs/metrics.md` (docs lists families, not all concrete names). This file — not `docs/metrics.md` — is the ground-truth emitted set for Task 5.

- [ ] **Step 5: Commit the harness**

```bash
git add internal/powerflex/dump_metrics_test.go
git commit -m "test(audit): add temporary emitted-metric dump harness"
```

---

## Task 5: WS3 — Grafana dashboard cross-check

**Files:**
- Create/append: `docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md`
- Inputs: `/tmp/dash_metrics.txt` (Task 3), `/tmp/emitted_metrics.txt` (Task 4)

- [ ] **Step 1: Compute dead panels (referenced but never emitted)**

Run:
```bash
grep -oE 'pflex_[a-z0-9_]+' /tmp/emitted_metrics.txt | sort -u > /tmp/emitted_names.txt
comm -23 /tmp/dash_metrics.txt /tmp/emitted_names.txt
```
Expected: a list (possibly empty) of metric names referenced by dashboards that the exporter never emits. Each is a **dead panel** candidate. For each, grep `grafana/` to find the exact file+panel: `grep -rl '<name>' grafana/`.

- [ ] **Step 2: Compute uncovered metrics (emitted but no panel)**

Run: `comm -13 /tmp/dash_metrics.txt /tmp/emitted_names.txt`
Expected: emitted metrics with no dashboard reference — **reported only**, not fixed.

- [ ] **Step 3: Triage dead-panel candidates against the trust rule**

For each dead-panel name: decide whether it is (a) a typo/renamed metric in the dashboard (Tier 1 fix — correct the dashboard expr to the real emitted name), or (b) a metric the exporter *should* emit but doesn't (Tier 3 coverage gap — report, do not invent). Distinguish by checking `git log -p` / `docs/metrics.md` for a rename, and by whether a sibling emitted name obviously corresponds.

- [ ] **Step 4: Record WS3 findings**

Append a `## WS3 — Grafana dashboard validation` section to the findings doc: dead-panel table (name → file → verdict → tier), uncovered-metric list. Use the report template in Task 8.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md
git commit -m "docs(audit): WS3 dashboard cross-check findings"
```

---

## Task 6: WS1 — Correctness audit (endpoints + payloads + fields)

**Files:**
- Append: findings doc
- Inputs: `extract_swagger.py` output for the 4 in-scope specs; Go source in `internal/powerflex/`

- [ ] **Step 1: Dump the in-scope swagger surfaces**

Run:
```bash
for f in 11227-4.5.5-Manager_v4-8-0 11227-5.0.0 11231-4.5.5-Manager_v4-8-0 11231-5.0.0; do
  python3 scripts/audit/extract_swagger.py docs/swagger/$f.json > /tmp/swag_$f.txt
done
wc -l /tmp/swag_*.txt
```
Expected: four non-trivial files.

- [ ] **Step 2: Audit the auth contract**

Compare `internal/powerflex/auth.go` against `/tmp/swag_11227-5.0.0.txt` and `...4.5.5...txt`:
- Paths: `/rest/auth/login`, `/rest/auth/update-token` — confirm both exist with method POST.
- Request: confirm body field names (username/password for login; refresh token for update-token) match the spec request schema.
- Response: confirm the token field names our code reads match the spec's 200 response fields.
Record each as MATCH / MISMATCH(reality-wins) / BUG with file:line evidence.

- [ ] **Step 3: Audit the Gen1 `/api/instances` + `querySelectedStatistics` contract**

In `/tmp/swag_11231-4.5.5-Manager_v4-8-0.txt`, locate `querySelectedStatistics` and the instances paths (likely templated, e.g. `/api/types/{type}/instances` or `/api/instances`). Compare against `internal/powerflex/client.go` (request construction) and `querySelectedStatistics.json` (the embedded request body). Confirm: endpoint path/method, the request body shape (selected-statistics list form), and that the stat names we request are accepted properties. Record verdicts.

- [ ] **Step 4: Audit the Gen2 dtapi `metrics/query` contract**

Locate the dtapi `metrics/query` operation in `/tmp/swag_11231-5.0.0.txt`. Compare against `internal/powerflex/statistics_v5.go`. **CRITICAL:** if the spec documents `metrics` as a string, that is the known-wrong contract — record it as a `MISMATCH(reality-wins): spec documents string, live dtapi requires JSON array; code is correct, do NOT change` note, citing the CLAUDE.md rationale. Confirm `resource_type` single-value-per-call and response field names map to `deriveSamplesV5`.

- [ ] **Step 5: Audit response struct field names (mechanical)**

Run: `grep -rhoE '`+"`"+`json:"[^"]+"'+"`"+'' internal/powerflex/*.go | sed -E 's/.*json:"([^",]+).*/\1/' | sort -u > /tmp/go_json_fields.txt`
Cross-check the high-value identity/stat field names in `/tmp/go_json_fields.txt` against the spec response `fields:` lines from Step 1. Flag any field our code reads that the spec does not define (potential typo or version drift) and apply the trust rule.

- [ ] **Step 6: Record WS1 findings + commit**

Append `## WS1 — Correctness audit` (per-endpoint verdict table). Then:
```bash
git add docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md
git commit -m "docs(audit): WS1 endpoint/payload/field correctness findings"
```

---

## Task 7: WS2 — Coverage gap analysis

**Files:**
- Append: findings doc

- [ ] **Step 1: List documented object types vs collected types**

From the in-scope Block API specs, list the instance/object types and statistics the API exposes. Compare against what we collect (`metricPrefix`, `labelBuildersGen1`/`labelBuildersGen2`, `querySelectedStatistics.json`, `v5Metrics` in `statistics_v5.go`). Run `grep -n 'metricPrefix\|labelBuildersGen' internal/powerflex/metrics.go` and `grep -n 'v5Metrics\|v5ResourceType' internal/powerflex/statistics_v5.go` to enumerate current coverage.

- [ ] **Step 2: Rank gaps by monitoring value**

For each documented-but-uncollected type/stat, assign priority (High/Med/Low) by operational value (capacity, latency, health > cosmetic/config fields). Do **not** implement — this is a reported backlog.

- [ ] **Step 3: Record WS2 findings + commit**

Append `## WS2 — Coverage gaps (reported, not implemented)` with a priority-ranked table.
```bash
git add docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md
git commit -m "docs(audit): WS2 coverage-gap backlog"
```

---

## Task 8: Assemble the findings report header + severity ranking

**Files:**
- Modify: `docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md`

- [ ] **Step 1: Prepend the report header**

Ensure the findings doc opens with this structure (sections WS1/WS2/WS3 already appended):

```markdown
# Swagger Validation Audit — Findings

**Date:** 2026-06-14
**Spec:** docs/superpowers/specs/2026-06-14-swagger-validation-audit.md
**Trust rule:** reality (tests/live behavior/CLAUDE.md) outranks swagger spec.

## Summary
| ID | Workstream | Severity | Tier | One-line |
|----|-----------|----------|------|----------|
| ... | WS1/2/3 | High/Med/Low | 1/2/3 | ... |

## Tier legend
- Tier 1: safe fix, applied directly (dead panel, doc mismatch).
- Tier 2: contract/code bug — code disagrees with BOTH spec and reality.
- Tier 3: reported only (spec-is-wrong notes, coverage gaps, ambiguous).
```

- [ ] **Step 2: Populate the summary table** from the WS1/WS2/WS3 sections — every finding gets one row with a stable ID (e.g. `WS3-01`).

- [ ] **Step 3: Self-check the report** — every dead panel, every endpoint, every gap appears in the summary table; no "TBD".

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md
git commit -m "docs(audit): assemble findings report + severity summary"
```

---

## Task 9: Apply Tier-1 fixes (dead panels, doc mismatches)

**Files:**
- Modify: `grafana/gen1/*.json`, `grafana/gen2/*.json`, `docs/metrics.md`, `docs/dashboards.md` (only those flagged)

- [ ] **Step 1: Fix each dead-panel expr**

For every WS3 Tier-1 dead panel (a dashboard expr referencing a renamed/typo'd metric), edit the dashboard JSON to the correct emitted name. Show the before/after in the commit. Validate JSON after each edit: `python3 -c "import json;json.load(open('<file>'))"` → no error.

- [ ] **Step 2: Re-run the cross-check to confirm zero dead panels remain**

Run:
```bash
python3 scripts/audit/extract_dashboards.py grafana | sort -u > /tmp/dash_metrics.txt
comm -23 /tmp/dash_metrics.txt /tmp/emitted_names.txt
```
Expected: empty output (every dashboard metric now exists) — OR only names confirmed as Tier-3 coverage gaps (documented in findings as intentionally unfixed). State which.

- [ ] **Step 3: Fix doc mismatches** in `docs/metrics.md` / `docs/dashboards.md` per WS1/WS3 findings (keep docs current — standing rule).

- [ ] **Step 4: Validate + commit**

Run: `make ci`
Expected: PASS (fmt, vet, lint, `go test -race`, govulncheck all green). The semgrep write-hook must not block — if it flags a JSON/doc edit, restructure rather than suppress.
```bash
git add grafana/ docs/metrics.md docs/dashboards.md
git commit -m "fix(grafana,docs): correct dead panels and doc mismatches from audit"
```

---

## Task 10: Apply Tier-2 fixes (real contract bugs) — conditional

**Files:**
- Modify: `internal/powerflex/*.go` (only where Step-confirmed)
- Test: `internal/powerflex/*_test.go`

> Skip this task entirely if WS1 found no Tier-2 bugs (code disagrees with BOTH spec and reality). The expected outcome of a healthy codebase is zero Tier-2 findings.

- [ ] **Step 1: For each Tier-2 finding, write a failing test**

Add a test in the appropriate `_test.go` that asserts the correct contract behavior (the one matching reality+spec). Mirror the existing mock-gateway/fixture style.

Run: `go test ./internal/powerflex/ -run <TestName> -v`
Expected: FAIL (proves the bug).

- [ ] **Step 2: Apply the minimal code fix**

Edit the offending `.go` file. **Never** change the dtapi `metrics` array form or re-add a 5xx retry clause (both are documented reality-wins constraints).

- [ ] **Step 3: Re-run the test**

Run: `go test ./internal/powerflex/ -run <TestName> -v`
Expected: PASS.

- [ ] **Step 4: Full gate + commit (per fix)**

Run: `make ci`
Expected: PASS.
```bash
git add internal/powerflex/
git commit -m "fix(powerflex): <specific contract bug> (audit WS1-NN)"
```

---

## Task 11: Clean up harness, finalize docs, final gate

**Files:**
- Remove or guard: `internal/powerflex/dump_metrics_test.go`
- Modify (if needed): `CLAUDE.md`, `README.md`, MkDocs nav for the findings doc

- [ ] **Step 1: Decide the dump harness fate**

If `TestDumpEmittedMetricNames` has lasting value, keep it but make it skip by default: add `if testing.Short() { t.Skip("audit-only dump") }` at the top, or gate behind an env var `if os.Getenv("PFLEX_DUMP_METRICS") == "" { t.Skip(...) }`. Otherwise remove the file: `git rm internal/powerflex/dump_metrics_test.go`.

- [ ] **Step 2: Update CLAUDE.md if the audit changed any documented constraint** (e.g. a corrected endpoint). If nothing changed semantically, leave it. Add a one-line pointer to the findings doc under docs if the project indexes audits.

- [ ] **Step 3: Build docs site to confirm no broken nav**

Run: `uvx --with mkdocs-material --with pymdown-extensions mkdocs build --strict`
Expected: builds clean (the spec/plan/findings live under `docs/superpowers/` — confirm they don't break `--strict`; if they're outside nav, that's fine, `--strict` fails only on referenced-but-missing pages).

- [ ] **Step 4: Final full gate**

Run: `make ci`
Expected: PASS.

- [ ] **Step 5: Commit + summarize**

```bash
git add -A
git commit -m "chore(audit): finalize swagger validation audit (harness cleanup, docs)"
```

Then report to the user: counts of findings by tier, what was fixed, what was reported-only (coverage gaps + spec-is-wrong notes), and the path to the findings doc.

---

## Self-Review (completed by plan author)

**Spec coverage:** WS1 → Task 6; WS2 → Task 7; WS3 → Tasks 3–5; trust hierarchy → enforced in Tasks 5/6/10; 3-spec scope → Task 6 Step 1; pinned reference → Task 1; fix tiers → Tasks 9 (T1), 10 (T2), reporting (T3) throughout; deliverable doc → Tasks 5–8; CI/semgrep gate → Tasks 9–11; docs-current rule → Tasks 9, 11. No spec requirement is unmapped.

**Placeholder scan:** The only intentional `TODO(adapt)` is in Task 4 Step 2, where the engineer must mirror `collector_test.go`'s mock harness — Step 1 forces reading that file first and the skeleton shows the exact gather/print logic. Acceptable because the harness constructor names are codebase-specific and must be read, not guessed.

**Type consistency:** `/tmp/emitted_names.txt` (Task 5 Step 1) is reused by name in Tasks 5, 9. `/tmp/dash_metrics.txt` produced in Task 3, regenerated in Task 9 Step 2. Script paths and the findings-doc path are identical across all tasks.
