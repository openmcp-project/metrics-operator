# Remote Cluster Access

## RemoteClusterAccess

Used by `Metric` and `ManagedMetric` to monitor resources in a remote cluster. Two authentication methods are supported.

### Option 1: Service Account Token (recommended)

Uses OIDC-based projected tokens — suitable for service mesh or in-cluster setups.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: RemoteClusterAccess
metadata:
  name: remote-cluster
  namespace: <monitoring-namespace>
spec:
  remoteClusterConfig:
    clusterSecretRef:
      name: remote-cluster-secret
      namespace: <secret-namespace>
    serviceAccountName: <service-account-name>
    serviceAccountNamespace: <service-account-namespace>
```

The secret referenced by `clusterSecretRef` must exist on the cluster running the operator and contain:

| Key | Description |
|---|---|
| `host` | API server endpoint of the remote cluster |
| `caData` | CA bundle (base64-encoded) |
| `audience` | Token audience for projected service account token |

You must also configure [RBAC](rbac.md) for the service account on the remote cluster.

### Option 2: Kubeconfig Secret

Provide an existing kubeconfig directly.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: RemoteClusterAccess
metadata:
  name: remote-cluster
  namespace: <monitoring-namespace>
spec:
  kubeConfigSecretRef:
    name: remote-kubeconfig-secret
    namespace: <secret-namespace>
    key: kubeconfig
```

---

## FederatedClusterAccess

Discovers multiple clusters dynamically by watching a Kubernetes resource type. Used by `FederatedMetric` and `FederatedManagedMetric`.

### kubeConfigPath

Reads the kubeconfig from a string or object field on the discovered resource:

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: FederatedClusterAccess
metadata:
  name: federate-ca-sample
  namespace: default
spec:
  target:
    kind: ControlPlane
    group: core.orchestrate.cloud.sap
    version: v1beta1
  kubeConfigPath: spec.target.kubeconfig
```

### secretRefPath

Reads the kubeconfig from a `SecretReference` object on the discovered resource. `namespace` defaults to the discovered resource's namespace; `key` defaults to `kubeconfig`.

```yaml
apiVersion: metrics.openmcp.cloud/v1alpha1
kind: FederatedClusterAccess
metadata:
  name: federate-ar-sample
  namespace: default
spec:
  target:
    kind: AccessRequest
    group: clusters.openmcp.cloud
    version: v1alpha1
  secretRefPath: status.secretRef
```

### Filtering Targets

Use label selectors, field selectors, and namespace scoping to restrict which discovered resources are used as cluster sources:

```yaml
spec:
  target:
    kind: ControlPlane
    group: core.orchestrate.cloud.sap
    version: v1beta1
  namespace: co-system
  labelSelector: "environment=production"
  fieldSelector: "spec.region=us-west"
  kubeConfigPath: spec.target.kubeconfig
```

> Namespace scoping only applies to namespaced target resources.
