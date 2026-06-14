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
