# Swagger Validation Audit — Design Spec

**Date:** 2026-06-14
**Status:** Approved (design); pending implementation plan
**Topic:** Validate the pflex_exporter implementation against the official Dell PowerFlex
swagger specs in `docs/swagger/`, including the Grafana dashboards.

## Goal

Run a one-time, human-readable audit of the exporter against the official Dell PowerFlex
REST API swagger specifications, then fix the findings. Three workstreams:

1. **Correctness audit** — endpoints, request payloads, query params, and response field
   names we use match the specs.
2. **Coverage gap analysis** — object types / statistics / fields the specs document that
   we do not collect; reported as a prioritized list (not auto-added).
3. **Grafana dashboard validation** — every PromQL metric referenced by the 16 dashboards
   is actually emitted; flag dead panels and uncovered metrics.

This is an audit + fix effort, **not** new automated tooling. No CI regression guard is
in scope.

## Non-goals

- No automated swagger-vs-code CI check / Go contract test (explicitly deferred).
- No auto-adding of newly-discovered metrics or object types — gaps are reported; the
  maintainer chooses what to implement.
- No changes to the 8 out-of-scope specs' surface area (see Scope).

## Scope

Only **3 of the 11** swagger files are in play — the exporter never calls the
Manager/SSO/Alerts/Events/Installer/Notifier APIs.

**In scope:**

| File | Spec | Relevance |
|------|------|-----------|
| `11227-4.5.5-Manager_v4-8-0.json` | auth | `/rest/auth/login`, `/rest/auth/update-token` |
| `11227-5.0.0.json` | auth | same, Gen2 |
| `11231-4.5.5-Manager_v4-8-0.json` | Gen1 Block API | `/api/instances`, `querySelectedStatistics` |
| `11231-5.0.0.json` | Gen2 Block Storage API | `/api/instances`, dtapi `metrics/query` |

**Out of scope (documented as such in the audit):** `11222` (Alerts), `11224` (Events),
`11226` (Notifier), `11228` (SSO), `11229` (Installer), `11230` (AsmManager),
`7225` (Manager REST). We issue no calls against these.

## Key principle — trust hierarchy (load-bearing)

The swagger specs are **not** unconditionally ground truth. CLAUDE.md records that the
PowerFlex 5.0 PDF documented the dtapi `metrics` field as a comma-separated string, but
the live dtapi returns HTTP 500 for that and only accepts a JSON array — "fixing to match
spec" would reintroduce the v0.6.2 regression.

Conflict resolution order:

```
passing tests + documented live-cluster behavior + CLAUDE.md notes  >  swagger spec
```

- Where code disagrees with the spec **but agrees with reality** → record a
  "spec-is-wrong-here" note. **Do not change code.**
- Where code disagrees with **both** spec and reality → that is a real bug; fix it.
- Where spec and reality agree and code matches → no action.

## Approach — Hybrid (chosen over pure-read and pure-script)

Script the mechanical, high-volume comparisons; reserve human/LLM reading for the handful
of semantic contracts.

- **Scripted (precise, no hallucination):** extract URL paths + response schema field
  names from the swagger JSON; extract endpoints, unmarshalled struct fields, and emitted
  metric names from the Go code/fixtures; mechanical set-diff. Especially the
  dashboard-metric ↔ emitted-metric cross-check.
- **Human/LLM read (judgment):** auth flow, Gen1 `querySelectedStatistics` request body,
  Gen2 dtapi `metrics/query` payload shape (the array nuance).

Rejected: pure read-through (huge specs → 920 KB Gen1 file, hand-diffing invites
hallucinated findings); fully scripted (misses semantic contracts).

## Workstreams

### WS1 — Correctness audit
For each in-scope endpoint we call (`auth/login`, `auth/update-token`, `/api/instances`,
Gen1 `querySelectedStatistics`, Gen2 dtapi `metrics/query`): confirm path, method, query
params, request body shape, and the response field names our Go structs (`models/`,
`metrics.go`, `statistics_v5.go`, `state.go`) unmarshal. Field-name diff is scripted;
payload semantics read by hand. Apply the trust hierarchy to every discrepancy.

### WS2 — Coverage gap analysis
Enumerate object types / statistics / fields the in-scope specs document that we do not
collect. Produce a prioritized "missing" list ranked by monitoring value (not mere
presence). Report only — no auto-implementation.

### WS3 — Grafana dashboard validation
Derive the **canonical emitted-metric set** by running the collector against the
`internal/powerflex/testdata/` fixtures and dumping sorted sample names (ground truth, not
eyeballed from source). Set-diff against every metric referenced in the 16
`grafana/gen1/` + `grafana/gen2/` dashboards. Two outputs:
- **Dead panels** — dashboard references a metric never emitted → safe fix.
- **Uncovered metrics** — emitted but no panel → reported.

## Deliverable

A single severity-ranked audit document:
`docs/superpowers/specs/2026-06-14-swagger-validation-audit-findings.md`
(or a clearly-named report file), with findings grouped by workstream and tagged by fix
tier.

## Fix tiers (gated)

- **Tier 1 — safe, applied directly:** dead dashboard panels, doc / `docs/metrics.md`
  mismatches, comment fixes.
- **Tier 2 — contract/code, applied where reality+spec agree and code is wrong:** real
  bugs only.
- **Tier 3 — reported, not changed:** spec-is-wrong notes; coverage gaps; anything
  ambiguous.

Every applied fix is gated by `make ci` (gofmt, `go vet`, golangci-lint, `go test -race`,
govulncheck) and the semgrep write-hook (no inline suppression — restructure instead).
Docs kept current per the standing rule (README, MkDocs site, CLAUDE.md, godoc).

## Pinned reference

`docs/swagger/` is currently untracked. Commit the 3 in-scope files (and, optionally, the
rest) as the pinned reference so the audit is reproducible against exact byte-for-byte
contracts. The 920 KB Gen1 Block API file is large but it is the contract.

## Success criteria

- Audit doc enumerates every in-scope endpoint with a match/mismatch verdict and trust
  verdict.
- Dashboard cross-check produces a complete dead-panel + uncovered-metric list against the
  generated emitted-metric set.
- All Tier-1 and confirmed Tier-2 fixes applied with `make ci` green and semgrep clean.
- No code change reintroduces a known spec-is-wrong contract (dtapi array form preserved).
