# RBAC Configuration

The operator needs read permissions for each resource type it monitors. Grant these via a `ClusterRole` and `ClusterRoleBinding`.

## Example

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: metrics-operator-role
rules:
- apiGroups:
  - "example.group"
  resources:
  - "exampleresources"
  verbs:
  - get
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: metrics-operator-rolebinding
subjects:
- kind: ServiceAccount
  name: metrics-operator-sa
  namespace: <operator-namespace>
roleRef:
  kind: ClusterRole
  name: metrics-operator-role
  apiGroup: rbac.authorization.k8s.io
```

Replace `<operator-namespace>` with the namespace the operator is deployed in. Adjust `apiGroups` and `resources` to match the resources you want to monitor.

```bash
kubectl apply -f rbac-config.yaml
```

## Remote Clusters

For [RemoteClusterAccess](remote-cluster-access.md) using service account token authentication, apply equivalent RBAC on each remote cluster using the configured `serviceAccountName`.

Update these roles whenever you add new resource types to monitor.
