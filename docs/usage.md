# Usage

## Metric

Monitors any Kubernetes resource by GroupVersionKind. Supports custom dimensions (called `projections`) to extract labels, annotations, or field values.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: Metric
metadata:
  name: metric-pod-count
spec:
  name: metric-pod-count
  description: Pods
  target:
    kind: Pod
    group: ""
    version: v1
  interval: "1m"
  projections:
    - name: pod-namespace
      fieldPath: "metadata.namespace"
```

See [Dimensions Configuration](dimensions-configuration.md) for full projection/dimension options.

## ManagedMetric

Monitors Crossplane managed resources (CRDs with categories "crossplane" and "managed"). By default exports `status.conditions`-based dimensions.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: ManagedMetric
metadata:
  name: managed-metric
spec:
  name: managed-metric
  description: Status metric created by an Operator
  target:
    kind: Release
    group: helm.crossplane.io
    version: v1beta1
  interval: "1m"
```

## FederatedMetric

Monitors resources across multiple clusters via a `FederatedClusterAccess`. Supports projections and filtering by cluster.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: FederatedMetric
metadata:
  name: xfed-prov
spec:
  name: xfed-prov
  description: crossplane providers
  target:
    kind: Provider
    group: pkg.crossplane.io
    version: v1
  interval: "1m"
  projections:
    - name: package
      fieldPath: "spec.package"
  federateClusterAccessRef:
    name: federate-ca-sample
    namespace: default
```

## FederatedManagedMetric

Monitors Crossplane managed resources across all clusters. Requires a `FederatedClusterAccess`. Resources must have CRD categories "crossplane" and "managed".

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: FederatedManagedMetric
metadata:
  name: xfed-managed
spec:
  name: xfed-managed
  description: crossplane managed resources
  interval: "1m"
  federateClusterAccessRef:
    name: federate-ca-sample
    namespace: default
```

## valueFrom: Gauge Value from a Resource Field

By default the gauge value equals the number of resources sharing a given dimension combination. Use `valueFrom` to set the gauge value from a field in the resource itself (e.g., a creation timestamp or replica count).

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: Metric
metadata:
  name: deployment-age
spec:
  name: deployment_age_seconds
  target:
    kind: Deployment
    group: apps
    version: v1
  interval: "1m"
  valueFrom:
    fieldPath: "metadata.creationTimestamp"
    type: timestamp       # "integer" or "timestamp" (RFC3339 → Unix seconds)
    aggregation: max      # "sum" (default), "max", "min", or "mean"
  projections:
    - name: namespace
      fieldPath: "metadata.namespace"
    - name: name
      fieldPath: "metadata.name"
```

See [Dimensions Configuration](dimensions-configuration.md#setting-the-gauge-value-from-a-resource-field-valuefrom) for full `valueFrom` details.

## Projection Default Values

Projections support a `default` value used when the field is absent from the resource. The `fieldType` should match the field type to avoid type mismatches.

```yaml
projections:
  - name: pod-namespace
    fieldPath: "status.conditions[?(@.type=='Healthy')].status"
    fieldType: "primitive"
    default: "unknown"
```

## DataSink Reference

All metric types support `dataSinkRef` to select which DataSink to use. If omitted, the operator uses the DataSink named `default` in its namespace.

```yaml
spec:
  dataSinkRef:
    name: my-datasink
```

See [DataSink Configuration](metrics-export.md) for setup details.
