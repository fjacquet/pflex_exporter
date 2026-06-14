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
