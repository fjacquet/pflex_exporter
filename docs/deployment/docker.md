# Docker

## Image

Multi-arch images are published to GHCR with SBOM and provenance attestations:

```bash
docker pull ghcr.io/fjacquet/pflex_exporter:0.3.0   # or :latest
```

## Run

Mount a config and pass cluster secrets via the environment (referenced as
`${PFLEX1_PASSWORD}` in the config):

```bash
docker run -d --name pflex_exporter \
  -p 2113:2113 \
  -e PFLEX1_PASSWORD='your-monitor-password' \
  -v "$PWD/config.yaml:/etc/pflex_exporter/config.yaml:ro" \
  ghcr.io/fjacquet/pflex_exporter:0.3.0
```

The image runs as a non-root user (`uid 10001`) and the binary embeds the statistics
query, so no extra files are needed at runtime.

### Logging

Logs always go to **stdout** (captured by `docker logs` / `docker compose logs`). When
`server.logName` in `config.yaml` is also set to a file path, that file is written *in
addition* to stdout. The shipped config uses the absolute path
`/var/log/pflex_exporter/pflex-exporter.log`; the image pre-creates that directory owned
by `uid 10001`, and the compose stack persists it in the `pflex_logs` named volume. If the
path is not writable, the exporter logs a warning and continues on stdout-only rather than
crashing. Set `logName: ""` to disable the file entirely.

## Compose stack

`docker-compose.yml` brings up the exporter together with Prometheus, Grafana (with the
bundled dashboards auto-provisioned), and an OpenTelemetry Collector. It **builds** the
exporter image locally:

```bash
PFLEX1_PASSWORD='your-monitor-password' docker compose up --build
```

If you'd rather run the **published** image instead of building, use
`docker-compose.ghcr.yml` — same stack, but the exporter is pulled from GHCR:

```bash
# :latest
PFLEX1_PASSWORD='your-monitor-password' docker compose -f docker-compose.ghcr.yml up -d
# pin a version
PFLEX_TAG=0.2.1 PFLEX1_PASSWORD='...' docker compose -f docker-compose.ghcr.yml up -d
# refresh images later
docker compose -f docker-compose.ghcr.yml pull
```

| Service | Port | Purpose |
|---|---|---|
| `pflex_exporter` | 2113 | `/metrics` + `/health` |
| `prometheus` | 9090 | scrapes the exporter (`prometheus.yml`) |
| `grafana` | 3000 | dashboards (login `admin` / `admin`), Prometheus datasource + `gen1`/`gen2` folders auto-provisioned |
| `otel-collector` | 4317 / 8889 | receives the OTLP push (when enabled) and re-exposes it |

Open Grafana at <http://localhost:3000> — the PowerFlex dashboards appear under the
**gen1** and **gen2** folders, already wired to the Prometheus datasource. To exercise the
OTLP path, set `opentelemetry.metrics.enabled: true` and `endpoint: "otel-collector:4317"`
in `config.yaml`.

### Grafana login

Credentials are set on the `grafana` service in `docker-compose.yml`:

```yaml
environment:
  - GF_SECURITY_ADMIN_USER=admin
  - GF_SECURITY_ADMIN_PASSWORD=admin
```

| | |
|---|---|
| URL | <http://localhost:3000> (or `http://<host>:3000`) |
| Username | `admin` |
| Password | `admin` |

Setting `GF_SECURITY_ADMIN_PASSWORD` explicitly also skips Grafana's first-login
forced-password-change screen.

**Changing the password.** `GF_SECURITY_ADMIN_PASSWORD` only applies when the Grafana
database is first created. The compose stack uses **ephemeral** Grafana storage (no named
volume), so `docker compose down && docker compose up -d` resets to `admin` / `admin` and
re-provisions the datasource and dashboards. To change it on an already-running instance:

```bash
docker compose exec grafana grafana cli admin reset-admin-password <newpass>
```

!!! warning
    These are local test-stack credentials. Change them (and avoid exposing port 3000)
    before using this stack anywhere shared.
