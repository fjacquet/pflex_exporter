# OpenTelemetry

The exporter is OpenTelemetry-first: in addition to the Prometheus `/metrics` endpoint it
can **push** metrics via OTLP, and optionally emit traces for diagnosing slow collection
cycles.

## Metric push

```yaml
opentelemetry:
  metrics:
    enabled: true
    endpoint: "otel-collector:4317"
    insecure: true
    interval: "10s"
```

When enabled, the exporter registers an observable gauge per metric name; a periodic
reader collects them every `interval` and pushes to the OTLP gRPC endpoint. The same
in-memory snapshot feeds both the OTLP push and the Prometheus scrape, so the two paths
always agree.

Every data point carries the same attributes as the Prometheus labels, including
`cluster`. Metric names match the Prometheus names (e.g. `pflex_cluster_iops`).

## Tracing

```yaml
opentelemetry:
  tracing:
    enabled: true
    endpoint: "otel-collector:4317"
    insecure: true
    samplingRate: 0.1
```

Tracing instruments each collection cycle (`collect.cycle`) and the per-request
PowerFlex API calls (`powerflex.request`), which helps pinpoint a slow gateway or a
cluster that is dragging out a cycle. Tracing is independent of the metric push — enable
either, both, or neither.

## Collector example

`otel-collector-config.yaml` (used by the Compose stack) accepts the OTLP push and
re-exposes the metrics for Prometheus:

```yaml
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
exporters:
  debug: {verbosity: normal}
  prometheus: {endpoint: 0.0.0.0:8889}
service:
  pipelines:
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [debug, prometheus]
```
