# Installation

Two installation methods are available.

---


## Option 1: Open Control Plane (OCP) (*recommended*)

If you use [Open Control Plane](https://open-control-plane.io), install the Metrics Operator as a managed service via the `MetricsOperator` resource. This makes the metrics-operator CRDs available on your control plane and runs the operator in a hidden workload cluster.

**Important:** The `metadata.name` of the `MetricsOperator` resource must match the name of your `ControlPlane` resource.

```yaml
apiVersion: metrics.services.open-control-plane.io/v1alpha1
kind: MetricsOperator
metadata:
  name: my-mcp                          # must match the ControlPlane name
  namespace: project-<project-name>--ws-<workspace-name>  # must match the ControlPlane namespace
spec:
  version: "v0.13.0"
```

Apply this to the OCP management cluster:

```bash
kubectl apply -f metricsoperator.yaml
```

The operator will proceed through phases: `Pending` → `Progressing` → `Ready`. Check status with:

```bash
kubectl get metricsoperator my-mcp -n project-<project-name>--ws-<workspace-name>
```

Once `Ready`, the Metrics Operator CRDs (`Metric`, `ManagedMetric`, `DataSink`, etc.) are available on the associated control plane. Continue with [DataSink Configuration](metrics-export.md) and [Usage](usage.md).


---

## Option 2: Helm

Deploy directly to any Kubernetes cluster:

```bash
helm upgrade --install metrics-operator oci://ghcr.io/openmcp-project/charts/metrics-operator \
  --namespace metrics-operator-system \
  --create-namespace \
  --version=<version>
```

Available versions are listed on [GitHub releases](https://github.com/openmcp-project/metrics-operator/releases).

### Post-install: Create a DataSink

After deploying, configure at least one DataSink so the operator knows where to send metrics:

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: DataSink
metadata:
  name: default
  namespace: metrics-operator-system
spec:
  connection:
    endpoint: "https://your-tenant.live.dynatrace.com/api/v2/otlp/v1/metrics"
  authentication:
    apiKey:
      secretKeyRef:
        name: dynatrace-credentials
        key: api-token
```

```bash
kubectl create secret generic dynatrace-credentials \
  --from-literal=api-token="<your-api-token>" \
  --namespace=metrics-operator-system
```

See [DataSink Configuration](metrics-export.md) for full details including mTLS and multiple sink setups.

