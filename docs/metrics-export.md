# Metrics Export

The Metrics Operator exports data in two complementary ways:

| Method             | Direction | Protocol         | What it covers                                                                         |
| ------------------ | --------- | ---------------- | -------------------------------------------------------------------------------------- |
| **DataSink**       | Push      | OTLP (HTTP/gRPC) | Business metrics you define (resource counts, dimensions)                              |
| **ServiceMonitor** | Pull      | Prometheus       | Business metrics + Operator internals (reconcile counts, errors, resource count gauge) |

---

## DataSink (OTLP Push)

DataSink is a custom resource defining where the operator pushes collected metrics. It supports HTTP(S) and gRPC(S) endpoints via [OpenTelemetry](https://opentelemetry.io) protocol.

### Specification

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: DataSink
metadata:
  name: default
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://your-tenant.live.dynatrace.com/api/v2/otlp/v1/metrics"
  authentication:       # optional
    apiKey:
      secretKeyRef:
        name: dynatrace-credentials
        key: api-token
```

### `spec.connection.endpoint`

The target OTLP endpoint URL. The protocol is inferred from the URL scheme:

| Scheme                 | Protocol |
| ---------------------- | -------- |
| `http://` / `https://` | HTTP(S)  |
| `grpc://` / `grpcs://` | gRPC(S)  |

### `spec.authentication`

Exactly one of `apiKey` or `certificate` must be specified (or omit the field entirely if the endpoint needs no auth).

#### API Key

```yaml
authentication:
  apiKey:
    secretKeyRef:
      name: dynatrace-credentials
      key: api-token
```

#### mTLS Certificate

```yaml
authentication:
  certificate:
    clientCertSecretKeyRef:
      name: opensearch-tls-creds
      key: client-cert
    clientKeySecretKeyRef:
      name: opensearch-tls-creds
      key: client-key
    caCertSecretKeyRef:          # optional
      name: opensearch-tls-creds
      key: ca-cert
```

Secrets must exist in the **same namespace** as the DataSink.

### Creating the Secret

```bash
kubectl create secret generic dynatrace-credentials \
  --from-literal=api-token="dt0c01.your-token-here" \
  --namespace=metrics-operator-system
```

### Default DataSink

If a metric resource omits `dataSinkRef`, the operator looks for a DataSink named `default`.

The namespace used for this lookup is resolved in this order:

1. `OPERATOR_CONFIG_NAMESPACE`
2. `POD_NAMESPACE`
3. `default` (hard fallback)

This lookup namespace is operator runtime configuration and is not derived from the Metric resource namespace.

Example: if the operator runs in `metrics-operator-system` and `POD_NAMESPACE=metrics-operator-system`, then a metric in any namespace still resolves `default` DataSink from `metrics-operator-system`.

If no `default` DataSink exists in the resolved namespace, the operator will not push metrics to any DataSink. The *pull* mode still works.

### Multiple DataSinks

You can define multiple DataSinks for different environments or teams and reference them explicitly:

```yaml
spec:
  dataSinkRef:
    name: prod-dynatrace
```

Example — environment-based:

```yaml
---
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: DataSink
metadata:
  name: dev-dynatrace
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://dev123.live.dynatrace.com/api/v2/otlp/v1/metrics"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-dev-credentials
        key: api-token
---
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: DataSink
metadata:
  name: prod-dynatrace
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://prod456.live.dynatrace.com/api/v2/otlp/v1/metrics"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-prod-credentials
        key: api-token
```

### Migration from Legacy Configuration

The old hardcoded secret approach (`dynatrace-credentials` in the operator namespace) is removed. Migrate by:

1. Create a DataSink resource pointing to your existing secret
2. Add `dataSinkRef` to your metric resources (or name the DataSink `default`)
3. Remove any hardcoded secret references from old configs

### Troubleshooting

```bash
# Check DataSink resources
kubectl get datasinks -n metrics-operator-system

# Describe a specific DataSink
kubectl describe datasink default -n metrics-operator-system

# Check operator logs
kubectl logs -n metrics-operator-system deployment/metrics-operator-controller-manager
```

| Error                          | Likely cause               | Fix                                           |
| ------------------------------ | -------------------------- | --------------------------------------------- |
| `Secret "..." not found`       | Missing secret             | Create the secret in the DataSink's namespace |
| `401 Unauthorized`             | Bad token                  | Verify token value and permissions            |
| `connection refused` / timeout | Wrong endpoint             | Check URL and network/firewall rules          |

### Best Practices

- Use separate secrets per environment; rotate tokens regularly
- Use descriptive DataSink names (`prod-dynatrace`, `dev-prometheus`)
- Monitor operator logs and DataSink status conditions for delivery failures

---

## ServiceMonitor (Prometheus Scrape)

The operator exposes a standard [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime) `/metrics` endpoint (HTTPS, port `https`) that Prometheus can scrape. This covers operator internals such as reconcile durations and error counts, and also exposes each business metric defined via `spec.name` as its own named Prometheus gauge.

### Exposed metrics

| Metric | Type | Description |
| --- | --- | --- |
| `<spec.name>` | Gauge | Named gauge with individual projection labels. `namespace` identifies the metric CR's namespace; a projection named `namespace` is exposed as `resource_namespace`. |

Standard controller-runtime metrics (work queue depth, reconcile errors, etc.) are also available.

### Metric names and labels

Before reconciliation starts, the operator snapshots its registered metric names once. These names are reserved for operator telemetry. The snapshot includes collectors with no samples yet and conservatively reserves each name's `_bucket`, `_sum`, and `_count` variants to protect histogram and summary series. Names that collide after Prometheus name escaping are also rejected.

A collision sets `Ready=False` with reason `InvalidMetricName` on `Metric`, `ManagedMetric`, `FederatedMetric`, or `FederatedManagedMetric`. The condition message identifies the conflicting name. Choose a different `spec.name`; the rejected metric does not break scraping of other metrics.

For namespace projections, the scrape labels distinguish the metric CR from the observed resource:

```prometheus
deployment_age_seconds{namespace="monitoring",resource_namespace="team-a",name="web",...} 1790755226
deployment_age_seconds{namespace="monitoring",resource_namespace="team-b",name="web",...} 1790755230
```

Other projection names are unchanged. If both `namespace` and `resource_namespace` are projected with different values, recording fails rather than discarding either value. OTLP dimension names are unchanged.

Metrics sharing a `spec.name` may expose different projection label sets. Adding or removing a projection does not prevent subsequent samples from being recorded. Existing series are retained until the operator restarts; this change does not add series expiration. Invalid label names or recording failures are reported through the metric's failure condition rather than silently skipped.

### Enabling the ServiceMonitor

A ready-to-use `ServiceMonitor` manifest is included at [`config/prometheus/monitor.yaml`](../config/prometheus/monitor.yaml). It is disabled by default. To enable it, uncomment the prometheus line in `config/default/kustomization.yaml`:

```yaml
resources:
- ../crd
- ../rbac
- ../manager
- ../prometheus    # uncomment this line
```

The manifest selects the controller-manager service by `control-plane: controller-manager` and scrapes `/metrics` over HTTPS using the pod's service account token:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: controller-manager-metrics-monitor
  namespace: metrics-operator-system
spec:
  endpoints:
    - path: /metrics
      port: https
      scheme: https
      bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
      tlsConfig:
        insecureSkipVerify: true
  selector:
    matchLabels:
      control-plane: controller-manager
```

> **Prerequisite:** The [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) must be installed in the cluster for `ServiceMonitor` resources to be recognized.

### Applying manually (without kustomize)

```bash
kubectl apply -f config/prometheus/monitor.yaml -n metrics-operator-system
```
