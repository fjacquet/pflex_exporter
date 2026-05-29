# Configuration

The exporter is configured with a single YAML file (default example: `config.yaml`).

```yaml
server:
  host: "0.0.0.0"
  port: "2112"
  uri: "/metrics"
  logName: "/var/log/pflex_exporter/pflex-exporter.log" # absolute; "" -> stdout only (recommended under systemd/k8s)

collection:
  interval: "10s" # how often the background loop polls every cluster
  timeout: "8s"   # per-cluster collection timeout

opentelemetry:
  metrics:                       # OTLP metric push
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    interval: "10s"
  tracing:                       # optional diagnostic tracing
    enabled: false
    endpoint: "localhost:4317"
    insecure: true
    samplingRate: 0.1

clusters:
  - name: flex-cluster1
    gateway: <ip-or-host>        # PowerFlex 4.x: primary ingress IP (Manager UI)
    username: <monitor-user>
    password: "${FLEX1_PASSWORD}"
    insecureSkipVerify: true
```

## Sections

| Section | Key | Notes |
|---|---|---|
| `server` | `host`, `port`, `uri` | HTTP bind address and Prometheus metrics path. |
| `server` | `logName` | Log file path (use an **absolute** path so it resolves the same in containers); empty string logs to stdout (recommended under systemd/k8s). If the path is not writable, logging falls back to stdout with a warning instead of failing to start. |
| `collection` | `interval` | Background poll period for every cluster. Matches Prometheus scrape cadence well at `10s`–`30s`. |
| `collection` | `timeout` | Per-cluster timeout; a slow/unreachable cluster fails fast without blocking others. |
| `opentelemetry.metrics` | `enabled`, `endpoint`, `interval` | OTLP gRPC metric push. |
| `opentelemetry.tracing` | `enabled`, `endpoint`, `samplingRate` | OTLP gRPC tracing for diagnosing slow cycles. |
| `clusters[]` | `name` | Unique; becomes the `cluster` label/attribute on every metric. |
| `clusters[]` | `gateway`, `username`, `password` | Connection details. `insecureSkipVerify` accepts self-signed gateway certs. |

## Generations

There is nothing to configure per generation. The exporter inspects each cluster's
storage-pool layout at collection time and picks the right path automatically: Gen1
(mirroring) uses the `querySelectedStatistics` API; Gen2 (erasure coding) uses the v5
metrics API. The detected generation is published as
`pflex_cluster_generation{generation="gen1|gen2|unknown"}`. One process can monitor a
mix of Gen1 and Gen2 clusters.

## Secrets

Cluster passwords should not be written in plaintext. Two options:

- **Environment interpolation** — `password: "${FLEX1_PASSWORD}"` is replaced with the
  value of the `FLEX1_PASSWORD` environment variable at load time. A referenced but unset
  variable is a startup error (fail-loud).
- **File reference** — `passwordFile: /etc/pflex_exporter/flex1.pass` reads the password
  from a file (trimmed).

```yaml
clusters:
  - name: flex-cluster2
    gateway: flex-clu2-gw01
    username: flex-username
    passwordFile: /etc/pflex_exporter/flex2.pass
    insecureSkipVerify: true
```

## Hot reload

The configuration is reloaded without a restart on **SIGHUP** or when the config file
changes on disk. A new config is validated before it is applied — an invalid file is
rejected and the running configuration is left untouched. When the set of clusters
changes, the client pool is rebuilt.

```bash
kill -HUP $(pgrep pflex_exporter)     # or: systemctl reload pflex_exporter
```

## Validation

`pflex_exporter --config config.yaml` validates on startup: port ranges, durations,
unique non-empty cluster names, required cluster fields, and OTLP endpoints. Use
`--once` to run a single collection cycle and exit (useful for smoke tests), and
`--debug` for verbose logging.
