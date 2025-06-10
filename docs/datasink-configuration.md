# DataSink Configuration Guide

This guide provides comprehensive information about configuring and using DataSink custom resources with the Metrics Operator.

## Overview

DataSink is a custom resource that defines where and how the Metrics Operator should send collected metrics data. It provides a flexible, secure, and centralized way to configure data destinations, replacing the previous hardcoded secret approach.

## DataSink Resource Structure

### Complete Example

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: production-dynatrace
  namespace: metrics-operator-system
  labels:
    environment: production
    provider: dynatrace
spec:
  connection:
    endpoint: "https://abc12345.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
    insecureSkipVerify: false
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-production-credentials
        key: api-token
```

### Specification Fields

#### `spec.connection`

Defines the connection details for the data sink.

- **`endpoint`** (required): The target endpoint URL where metrics will be sent
  - For Dynatrace: `https://{your-environment-id}.live.dynatrace.com/api/v2/metrics/ingest`
  - For custom endpoints: Any valid HTTP/HTTPS or gRPC URL

- **`protocol`** (required): Communication protocol
  - `http`: HTTP/HTTPS protocol
  - `grpc`: gRPC protocol

- **`insecureSkipVerify`** (optional): Skip TLS certificate verification
  - `false` (default): Verify TLS certificates
  - `true`: Skip TLS verification (not recommended for production)

#### `spec.authentication`

Defines authentication mechanisms for the data sink.

##### API Key Authentication

Currently, the only supported authentication method is API key authentication:

- **`apiKey.secretKeyRef`** (required): Reference to a Kubernetes Secret containing the API key
  - **`name`**: Name of the Secret containing the API key
  - **`key`**: Key within the Secret that contains the API token

## Secret Configuration

The authentication secret must be created in the same namespace as the DataSink resource (typically the operator's namespace).

### Example Secret

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: dynatrace-production-credentials
  namespace: metrics-operator-system
type: Opaque
data:
  # Base64 encoded API token
  api-token: ZHQwYzAxLmFiY2RlZmdoaWprbG1ub3BxcnN0dXZ3eHl6MTIzNDU2Nzg5MA==
stringData:
  # Alternative: provide the token directly (will be base64 encoded automatically)
  # api-token: "dt0c01.abcdefghijklmnopqrstuvwxyz1234567890"
```

### Creating Secrets

You can create the secret using `kubectl`:

```bash
# Create secret with base64 encoding
kubectl create secret generic dynatrace-production-credentials \
  --from-literal=api-token="dt0c01.abcdefghijklmnopqrstuvwxyz1234567890" \
  --namespace=metrics-operator-system

# Or create from a file
echo -n "dt0c01.abcdefghijklmnopqrstuvwxyz1234567890" > /tmp/token
kubectl create secret generic dynatrace-production-credentials \
  --from-file=api-token=/tmp/token \
  --namespace=metrics-operator-system
```

## Using DataSink in Metrics

### Explicit Reference

Specify the DataSink to use in your metric resources:

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: Metric
metadata:
  name: pod-metrics
  namespace: default
spec:
  name: "kubernetes.pods.count"
  description: "Number of pods in the cluster"
  target:
    kind: Pod
    group: ""
    version: v1
  interval: "1m"
  dataSinkRef:
    name: production-dynatrace  # References the DataSink
```

### Default DataSink

If no `dataSinkRef` is specified, the operator automatically uses a DataSink named "default" in the operator's namespace:

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: Metric
metadata:
  name: pod-metrics-default
  namespace: default
spec:
  name: "kubernetes.pods.count.default"
  target:
    kind: Pod
    group: ""
    version: v1
  # No dataSinkRef - uses "default" DataSink automatically
```

## Multiple DataSinks

You can configure multiple DataSinks for different purposes:

### Environment-based DataSinks

```yaml
---
# Development environment DataSink
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: dev-dynatrace
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://dev123.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-dev-credentials
        key: api-token
---
# Production environment DataSink
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: prod-dynatrace
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://prod456.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-prod-credentials
        key: api-token
```

### Team-based DataSinks

```yaml
---
# Platform team DataSink
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: platform-metrics
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://platform.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: platform-dynatrace-credentials
        key: api-token
---
# Application team DataSink
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: app-metrics
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://apps.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: app-dynatrace-credentials
        key: api-token
```

## Supported Metric Types

All metric resource types support the `dataSinkRef` field:

### Metric

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: Metric
metadata:
  name: service-count
spec:
  name: "kubernetes.services.count"
  target:
    kind: Service
    group: ""
    version: v1
  dataSinkRef:
    name: platform-metrics
```

### ManagedMetric

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: ManagedMetric
metadata:
  name: crossplane-releases
spec:
  name: "crossplane.releases.count"
  kind: Release
  group: helm.crossplane.io
  version: v1beta1
  dataSinkRef:
    name: platform-metrics
```

### FederatedMetric

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: FederatedMetric
metadata:
  name: federated-providers
spec:
  name: "crossplane.providers.federated"
  target:
    kind: Provider
    group: pkg.crossplane.io
    version: v1
  federateClusterAccessRef:
    name: cluster-federation
    namespace: default
  dataSinkRef:
    name: platform-metrics
```

### FederatedManagedMetric

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: FederatedManagedMetric
metadata:
  name: federated-managed-resources
spec:
  name: "crossplane.managed.federated"
  federateClusterAccessRef:
    name: cluster-federation
    namespace: default
  dataSinkRef:
    name: platform-metrics
```

## Migration from Legacy Configuration

### Before (Deprecated)

Previously, the operator used hardcoded secret names:

```yaml
# This approach is no longer supported
apiVersion: v1
kind: Secret
metadata:
  name: dynatrace-credentials  # Hardcoded name
  namespace: metrics-operator  # Hardcoded namespace
type: Opaque
data:
  api-token: <base64-encoded-token>
```

### After (Current)

Now you must use DataSink resources:

```yaml
---
# 1. Create the DataSink
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: default
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://your-tenant.live.dynatrace.com/api/v2/metrics/ingest"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-credentials  # Any name you choose
        key: api-token
---
# 2. Create the secret with your chosen name
apiVersion: v1
kind: Secret
metadata:
  name: dynatrace-credentials
  namespace: metrics-operator-system
type: Opaque
data:
  api-token: <base64-encoded-token>
```

### Migration Steps

1. **Create DataSink resources** for each data destination you need
2. **Update metric resources** to reference the appropriate DataSink using `dataSinkRef`
3. **Remove old hardcoded secrets** (they are no longer used)
4. **Test your configuration** to ensure metrics are being sent correctly

## Troubleshooting

### Common Issues

#### DataSink Not Found

**Error**: `DataSink "default" not found in namespace "metrics-operator-system"`

**Solution**: Create a DataSink named "default" in the operator's namespace, or specify a `dataSinkRef` in your metric resources.

#### Secret Not Found

**Error**: `Secret "dynatrace-credentials" not found`

**Solution**: Ensure the secret referenced in `authentication.apiKey.secretKeyRef` exists in the same namespace as the DataSink.

#### Authentication Failed

**Error**: `401 Unauthorized` or similar authentication errors

**Solution**:
- Verify the API token is correct and has the necessary permissions
- Check that the token is properly base64 encoded in the secret
- Ensure the endpoint URL is correct for your data sink

#### Connection Failed

**Error**: `connection refused` or `timeout` errors

**Solution**:
- Verify the endpoint URL is correct and accessible
- Check network connectivity from the cluster to the data sink
- Verify firewall rules allow outbound connections

### Debugging Commands

```bash
# Check DataSink resources
kubectl get datasinks -n metrics-operator-system

# Describe a specific DataSink
kubectl describe datasink default -n metrics-operator-system

# Check DataSink status
kubectl get datasink default -n metrics-operator-system -o yaml

# Check secrets
kubectl get secrets -n metrics-operator-system

# Check operator logs
kubectl logs -n metrics-operator-system deployment/metrics-operator-controller-manager
```

### Validation

To verify your DataSink configuration is working:

1. **Check DataSink status**: Look for any error conditions in the DataSink status
2. **Monitor operator logs**: Check for connection or authentication errors
3. **Verify metric delivery**: Confirm metrics are appearing in your data sink
4. **Test connectivity**: Use tools like `curl` to test endpoint connectivity from within the cluster

## Best Practices

### Security

- **Use separate secrets** for different environments (dev, staging, production)
- **Rotate API tokens** regularly and update secrets accordingly
- **Use RBAC** to limit access to DataSink resources and secrets
- **Enable TLS verification** (`insecureSkipVerify: false`) in production

### Organization

- **Use descriptive names** for DataSinks (e.g., `production-dynatrace`, `dev-prometheus`)
- **Add labels** to DataSinks for better organization and filtering
- **Document your configuration** with annotations or external documentation
- **Use namespaces** to separate DataSinks by team or environment

### Monitoring

- **Monitor DataSink status** for connection issues
- **Set up alerts** for failed metric deliveries
- **Track metric volume** to ensure data is being sent as expected
- **Monitor operator health** to catch configuration issues early

## Advanced Configuration

### Custom Endpoints

DataSink supports any HTTP or gRPC endpoint:

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: custom-endpoint
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://custom-metrics.example.com/api/v1/metrics"
    protocol: "http"
  authentication:
    apiKey:
      secretKeyRef:
        name: custom-api-credentials
        key: token
```

### gRPC Configuration

For gRPC endpoints:

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: DataSink
metadata:
  name: grpc-endpoint
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "grpc://metrics-service.example.com:9090"
    protocol: "grpc"
  authentication:
    apiKey:
      secretKeyRef:
        name: grpc-credentials
        key: api-key
```

This comprehensive guide should help you configure and use DataSink resources effectively with the Metrics Operator.