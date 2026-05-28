# Quick Start

```bash
make cli
export FLEX1_PASSWORD='your-monitor-password'
./bin/pflex_exporter --config config.yaml
```

Then:

```bash
curl -s localhost:2112/metrics | grep '^pflex_up'
curl -s localhost:2112/health
```

You should see `pflex_up{cluster="flex-cluster1"} 1` once the first collection cycle
completes, and `/health` returns `200 OK`.

## One-shot mode

Run a single collection cycle and exit — handy for verifying connectivity and that the
expected metrics are produced, without starting the server loop:

```bash
./bin/pflex_exporter --config config.yaml --once --debug
```

## Local stack (Docker Compose)

Bring up the exporter alongside Prometheus and an OpenTelemetry Collector:

```bash
FLEX1_PASSWORD='your-monitor-password' docker compose up --build
```

- Exporter metrics: <http://localhost:2112/metrics>
- Prometheus: <http://localhost:9090>
- OTLP collector receives the push when `opentelemetry.metrics.enabled: true`.

## What to look at

- Per-cluster health: `pflex_up`, `pflex_last_scrape_timestamp_seconds`, `pflex_cluster_generation`.
- Capacity: `pflex_cluster_capacity_in_use_in_kb` vs `pflex_cluster_max_capacity_in_kb`.
- Performance: `pflex_cluster_iops{op="total"}`, `pflex_cluster_bandwidth_kb_per_second{op="total"}`.

See the [Metrics Reference](../metrics.md) for the full list and the
[Dashboards](../dashboards.md) page for ready-made Grafana panels.
