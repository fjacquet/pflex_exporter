# Quick Start

```bash
make cli
export PFLEX1_PASSWORD='your-monitor-password'
./bin/pflex_exporter --config config.yaml
```

Then:

```bash
curl -s localhost:9445/metrics | grep '^pflex_up'
curl -s localhost:9445/health
```

You should see `pflex_up{cluster="flex-cluster1"} 1` once the first collection cycle
completes, and `/health` returns `200 OK`.

## One-shot mode

Run a single collection cycle and exit — handy for verifying connectivity and that the
expected metrics are produced, without starting the server loop:

```bash
./bin/pflex_exporter --config config.yaml --once --debug
```

Useful flags:

- `--once` — run a single collection cycle, log the result, and exit (connectivity check).
- `--debug` — verbose logging, including per-collector failures. Combined with
  `--once`, it also prints **every collected sample** (sorted, exposition style)
  so you can diff a live cluster against the [Metrics Reference](../metrics.md).
- `--trace` — log every gateway API response body (method, URL, status, payload).
  Headers are never logged and the login/token-refresh responses are skipped
  entirely, so auth tokens cannot leak. Use it when a metric you expect is
  absent: the exporter never guesses values, so an unexpected payload shape shows
  up as a missing sample — the trace shows what the cluster actually returned.

Validating against a real cluster:

```bash
./bin/pflex_exporter --config config.yaml --once --debug --trace > validate.log
grep -v '^{' validate.log | sort > samples.txt   # every collected sample (compare with docs/metrics.md)
grep -F 'API trace' validate.log > trace.log     # raw API payloads for anything missing or suspicious
```

(Log lines are JSON objects on stdout, while the sample dump is plain exposition
lines, so the two are easy to separate.)

## Local stack (Docker Compose)

Bring up the exporter alongside Prometheus, Grafana (dashboards auto-provisioned), and an
OpenTelemetry Collector:

```bash
PFLEX1_PASSWORD='your-monitor-password' docker compose up --build
```

- Exporter metrics: <http://localhost:9445/metrics>
- Prometheus: <http://localhost:9090>
- Grafana: <http://localhost:3000> (login `admin` / `admin`; PowerFlex dashboards under the **gen1** / **gen2** folders)
- OTLP collector receives the push when `opentelemetry.metrics.enabled: true`.

To run the **published** image instead of building locally, use the pull-based stack:

```bash
PFLEX1_PASSWORD='your-monitor-password' docker compose -f docker-compose.ghcr.yml up -d
```

See [Docker deployment](../deployment/docker.md) for both stacks, image tags, and Grafana details.

## What to look at

- Per-cluster health: `pflex_up`, `pflex_last_scrape_timestamp_seconds`, `pflex_cluster_generation`.
- Capacity: `pflex_cluster_capacity_in_use_in_kb` vs `pflex_cluster_max_capacity_in_kb`.
- Performance: `pflex_cluster_iops{op="total"}`, `pflex_cluster_bandwidth_kb_per_second{op="total"}`.

See the [Metrics Reference](../metrics.md) for the full list and the
[Dashboards](../dashboards.md) page for ready-made Grafana panels.
