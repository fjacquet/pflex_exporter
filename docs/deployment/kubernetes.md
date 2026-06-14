# Kubernetes

Manifests live in `deploy/kubernetes/`, wired together with kustomize.

| File | Purpose |
|---|---|
| `configmap.yaml` | `config.yaml` (set clusters here; `logName: ""` for pod logs) |
| `secret.example.yaml` | cluster passwords (`PFLEX1_PASSWORD`, …) |
| `deployment.yaml` | hardened single-replica Deployment |
| `service.yaml` | ClusterIP exposing the `metrics` port (9445) |
| `servicemonitor.yaml` | optional Prometheus Operator scrape config |
| `kustomization.yaml` | ties the above together |

## Apply

```bash
# edit configmap.yaml (clusters) and secret.example.yaml (passwords) first
kubectl apply -k deploy/kubernetes/
kubectl rollout status deploy/pflex-exporter
```

## Design notes

- **Single replica.** The collector is a singleton — a second replica would double-poll
  every cluster (there is no leader election). The Deployment uses `strategy: Recreate`.
- **Security context.** Runs as non-root (`uid 10001`), `readOnlyRootFilesystem: true`,
  all capabilities dropped, `seccompProfile: RuntimeDefault`. A small `emptyDir` is
  mounted at `/tmp`.
- **Probes.** Liveness hits `/metrics` (process health, always 200 while serving);
  readiness hits `/health` (ready only once at least one cluster has been collected).
- **Secrets.** Passwords come from the Secret via `envFrom` and are interpolated into
  `config.yaml` as `${PFLEX1_PASSWORD}`. Prefer an external secret manager
  (External Secrets Operator, sealed-secrets, etc.) over committing the example Secret.

## Workload enrichment (optional)

Set `kubernetes.enabled: true` in `config.yaml` to label volume and SDC metrics with the
Kubernetes workloads behind them (namespace, PVC, PV, storage class, node) — see
[Configuration → Kubernetes enrichment](../getting-started/configuration.md#kubernetes-workload-enrichment).

When running in-cluster the exporter uses its pod service account, which needs **read-only**
access to PersistentVolumes and Nodes (both cluster-scoped):

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: pflex-exporter-enrichment
rules:
  - apiGroups: [""]
    resources: ["persistentvolumes", "nodes"]
    verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: pflex-exporter-enrichment
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: pflex-exporter-enrichment
subjects:
  - kind: ServiceAccount
    name: pflex-exporter
    namespace: <your-namespace>
```

The feature degrades to a no-op if the API is unreachable, so a missing binding only loses
the enrichment labels — core metrics keep flowing.

## Prometheus Operator

If you run the Prometheus Operator, uncomment `servicemonitor.yaml` in
`kustomization.yaml`. Otherwise scrape the Service directly with a `static_config` or
`kubernetes_sd_config` targeting the `metrics` port.
