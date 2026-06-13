# Configuration

The exporter is configured with a single YAML file (default example: `config.yaml`).

```yaml
server:
  host: "0.0.0.0"
  port: "2113"
  uri: "/metrics"
  logName: "/var/log/pflex_exporter/pflex-exporter.log" # absolute; "" -> stdout only (recommended under systemd/k8s)

collection:
  interval: "10s"            # how often the background loop polls every cluster
  timeout: "8s"              # per-cluster collection timeout
  maxConcurrentClusters: 0   # cap on clusters polled in parallel; 0 = unlimited
  slowResourceEveryN: 1      # decimation multiplier (Gen2); 1 = disabled
  slowResourceTypes: []      # e.g. ["DeviceGroup", "Sdt", "ProtectionDomain"]

kubernetes:                  # optional workload enrichment (portable; no-op when standalone)
  enabled: false
  refreshInterval: "60s"
  csiDriverName: "csi-vxflexos.dellemc.com"
  # kubeconfig: "/path/to/kubeconfig"   # optional; in-cluster / default rules used when empty

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
    password: "${PFLEX1_PASSWORD}"
    insecureSkipVerify: true
```

## Sections

| Section | Key | Notes |
|---|---|---|
| `server` | `host`, `port`, `uri` | HTTP bind address and Prometheus metrics path. |
| `server` | `logName` | Log file path (use an **absolute** path so it resolves the same in containers); empty string logs to stdout (recommended under systemd/k8s). If the path is not writable, logging falls back to stdout with a warning instead of failing to start. |
| `collection` | `interval` | Background poll period for every cluster. Matches Prometheus scrape cadence well at `10s`–`30s`. |
| `collection` | `timeout` | Per-cluster timeout; a slow/unreachable cluster fails fast without blocking others. |
| `collection` | `maxConcurrentClusters` | Cap on clusters polled in parallel per cycle. `0` (default) = unlimited. Set it when monitoring many clusters to smooth concurrent API load. |
| `collection` | `slowResourceEveryN`, `slowResourceTypes` | Decimation: collect the listed object types' statistics only every Nth cycle (reusing prior samples in between). See [Scaling](#scaling-large-estates). |
| `kubernetes` | `enabled`, `refreshInterval`, `csiDriverName`, `kubeconfig` | Optional workload enrichment. See [Kubernetes enrichment](#kubernetes-workload-enrichment). |
| `opentelemetry.metrics` | `enabled`, `endpoint`, `interval` | OTLP gRPC metric push. |
| `opentelemetry.tracing` | `enabled`, `endpoint`, `samplingRate` | OTLP gRPC tracing for diagnosing slow cycles. |
| `clusters[]` | `name` | Unique; becomes the `cluster` label/attribute on every metric. |
| `clusters[]` | `gateway`, `username`, `password` | Connection details. `insecureSkipVerify` accepts self-signed gateway certs. |

## Scaling large estates

Two knobs bound the API load the exporter places on PowerFlex when monitoring many
clusters or very large arrays:

- **`maxConcurrentClusters`** limits how many clusters are polled at once. With the
  default `0` every configured cluster is polled concurrently each cycle; set a positive
  cap (e.g. `4`) to spread the load.
- **Decimation** (`slowResourceEveryN` + `slowResourceTypes`) collects slow-changing
  object types less often. With `slowResourceEveryN: 6` and
  `slowResourceTypes: ["DeviceGroup", "Sdt", "ProtectionDomain"]`, those types' statistics
  are fetched on every 6th cycle and the previous samples are reused in between — their
  series stay continuous while their API queries drop by ~83%.

  Decimation is **Gen2-only**: Gen2 fetches statistics per resource type, so skipping a
  type saves a real API call. Gen1 returns all statistics in a single query, so there is
  nothing to skip — Gen1 clusters are always collected in full. Operational state and
  health metrics come from the instance list (fetched every cycle), so they remain fresh
  regardless of decimation.

## Kubernetes workload enrichment

When `kubernetes.enabled: true`, the exporter resolves Kubernetes workload context from
the cluster's PersistentVolumes and Nodes and adds it as labels:

| Metrics | Added labels |
|---|---|
| `pflex_volume_*` (performance & capacity) | `namespace`, `persistent_volume_claim`, `persistent_volume`, `storage_class` |
| `pflex_sdc_*` (performance) | `k8s_node` |

Volumes are matched to PVs through the PowerFlex CSI driver (`csiDriverName`, default
`csi-vxflexos.dellemc.com`): the PV's CSI `volumeHandle` (`<systemID>-<volumeID>`) maps to
the PowerFlex volume ID, and the PV's `claimRef` supplies the namespace and claim. SDCs are
matched to Nodes by IP address. The PV/Node caches refresh every `refreshInterval`.

This feature is **portable**. The exporter looks for an in-cluster service account, then a
`kubeconfig` (explicit path, `KUBECONFIG`, or `~/.kube/config`). If none is reachable, it
logs a warning and runs with enrichment disabled — the same binary works standalone, in a
VM, or in a pod. When enrichment is on, the added label keys are present on **every**
volume/SDC series (empty when a workload can't be resolved) so the metric label set stays
consistent. See [Deployment → Kubernetes](../deployment/kubernetes.md) for the RBAC the
service account needs.

## Generations

There is nothing to configure per generation. The exporter inspects each cluster's
storage-pool layout at collection time and picks the right path automatically: Gen1
(mirroring) uses the `querySelectedStatistics` API; Gen2 (erasure coding) uses the v5
metrics API. The detected generation is published as
`pflex_cluster_generation{generation="gen1|gen2|unknown"}`. One process can monitor a
mix of Gen1 and Gen2 clusters.

## Environment variables / .env

`gateway`, `username`, and `password` all support `${ENV_VAR}` interpolation.
A referenced but unset variable is a startup error (fail-loud, never a silent empty value).

`config.yaml` ships with all three fields parameterized as `${PFLEX1_GATEWAY}`,
`${PFLEX1_USERNAME}`, and `${PFLEX1_PASSWORD}`. This is the **single-cluster quickstart
convenience**: copy `.env.example` to `.env`, fill in the three values, and
`docker compose up` resolves them automatically.

```bash
cp .env.example .env
# edit PFLEX1_GATEWAY / PFLEX1_USERNAME / PFLEX1_PASSWORD
docker compose up -d
```

### .env loading

The `pflex_exporter` binary loads a `.env` file natively at startup — from the working
directory first, then next to the config file — so `cp .env.example .env` works for
bare-metal and systemd runs exactly like it does under docker compose.
Already-set environment variables **always take precedence** over `.env` values,
so secret injection (systemd `Environment=`, Kubernetes secrets, CI) can never be
shadowed by a stray file.

`config.yaml` is the **source of truth** and is always consumed; `.env` is only a
convenience layer. For production deployments (systemd, Kubernetes) set the variables in
your own secrets manager and inject them into the process environment.

### Multi-cluster

For more than one cluster, add one entry per cluster with either literal values or your
own env refs. The `PFLEX1_*` names are a single-cluster convention; you can choose any
names you like:

```yaml
clusters:
  - name: flex-cluster1
    gateway: "${PFLEX1_GATEWAY}"
    username: "${PFLEX1_USERNAME}"
    password: "${PFLEX1_PASSWORD}"
    insecureSkipVerify: true
  - name: flex-cluster2
    gateway: "${PFLEX2_GATEWAY}"
    username: "${PFLEX2_USERNAME}"
    password: "${PFLEX2_PASSWORD}"
    insecureSkipVerify: true
```

Pass the additional variables to the compose stack (e.g. in `.env` alongside
the `PFLEX1_*` entries) or inject them at the deployment level.

## Secrets

Cluster passwords should not be written in plaintext. Two options:

- **Environment interpolation** — `password: "${PFLEX1_PASSWORD}"` is replaced with the
  value of the `PFLEX1_PASSWORD` environment variable at load time. A referenced but unset
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
`--debug` for verbose logging — combined with `--once` it also prints every
collected sample (sorted, exposition style). Add `--trace` to log every gateway
API response body (auth responses are skipped, so tokens never reach the log);
see the [Quick Start](quickstart.md) for the full live-cluster validation recipe.
