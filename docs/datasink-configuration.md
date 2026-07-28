# DataSink Configuration Guide

DataSink is a custom resource defining where the Metrics Operator sends collected metrics. It supports HTTP(S) and gRPC(S) endpoints via [OpenTelemetry](https://opentelemetry.io) protocol.

## Specification

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

## Creating the Secret

```bash
kubectl create secret generic dynatrace-credentials \
  --from-literal=api-token="dt0c01.your-token-here" \
  --namespace=metrics-operator-system
```

## Default DataSink

If a metric resource omits `dataSinkRef`, the operator looks for a DataSink named `default` in its own namespace. Create one to keep metric definitions simple.

## Multiple DataSinks

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

## Migration from Legacy Configuration

The old hardcoded secret approach (`dynatrace-credentials` in the operator namespace) is removed. Migrate by:

1. Create a DataSink resource pointing to your existing secret
2. Add `dataSinkRef` to your metric resources (or name the DataSink `default`)
3. Remove any hardcoded secret references from old configs

## Troubleshooting

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
| `DataSink "default" not found` | No default DataSink exists | Create one named `default`                    |
| `Secret "..." not found`       | Missing secret             | Create the secret in the DataSink's namespace |
| `401 Unauthorized`             | Bad token                  | Verify token value and permissions            |
| `connection refused` / timeout | Wrong endpoint             | Check URL and network/firewall rules          |

## Best Practices

- Use separate secrets per environment; rotate tokens regularly
- Keep `insecureSkipVerify` disabled (not a supported field — TLS is always verified based on the endpoint scheme)
- Use descriptive DataSink names (`prod-dynatrace`, `dev-prometheus`)
- Monitor operator logs and DataSink status conditions for delivery failures
