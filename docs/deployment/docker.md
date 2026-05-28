# Docker

## Image

Multi-arch images are published to GHCR with SBOM and provenance attestations:

```bash
docker pull ghcr.io/fjacquet/pflex_exporter:0.1.0   # or :latest
```

## Run

Mount a config and pass cluster secrets via the environment (referenced as
`${FLEX1_PASSWORD}` in the config):

```bash
docker run -d --name pflex_exporter \
  -p 2112:2112 \
  -e FLEX1_PASSWORD='your-monitor-password' \
  -v "$PWD/config.yaml:/etc/pflex_exporter/config.yaml:ro" \
  ghcr.io/fjacquet/pflex_exporter:0.1.0
```

The image runs as a non-root user (`uid 10001`) and the binary embeds the statistics
query, so no extra files are needed at runtime.

## Compose stack

`docker-compose.yml` brings up the exporter together with Prometheus and an OpenTelemetry
Collector:

```bash
FLEX1_PASSWORD='your-monitor-password' docker compose up --build
```

| Service | Port | Purpose |
|---|---|---|
| `pflex_exporter` | 2112 | `/metrics` + `/health` |
| `prometheus` | 9090 | scrapes the exporter (`prometheus.yml`) |
| `otel-collector` | 4317 / 8889 | receives the OTLP push (when enabled) and re-exposes it |

To exercise the OTLP path, set `opentelemetry.metrics.enabled: true` and
`endpoint: "otel-collector:4317"` in `config.yaml`.
