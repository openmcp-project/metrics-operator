[![REUSE status](https://api.reuse.software/badge/github.com/SAP/metrics-operator)](https://api.reuse.software/info/github.com/SAP/metrics-operator)

# Metrics Operator

The Metrics Operator is a powerful tool designed to monitor and provide insights into the state, usage, patterns, and trends of distributed systems and their associated components.

## Table of Contents

- [Key Features](#key-features)
- [Installation](#installation)
- [Usage](#usage)
- [RBAC Configuration](#rbac-configuration)
- [Remote Cluster Access](#remote-cluster-access)
- [Data Sink Integration](#data-sink-integration)

## Key Features

- **Comprehensive Resource Tracking**: Quantifies and catalogs various resource types, providing a holistic view of resource distribution and utilization.
- **Multi-dimensional Analysis**: Examines specific attributes and dimensions of resources, generating nuanced metrics for deeper understanding of system behavior.
- **Comparative Analytics**: Enables side-by-side analysis of different resource configurations, highlighting patterns and potential imbalances in resource allocation.
- **Custom Component Focus**: Tailored to monitor and analyze complex, custom-defined resources across your infrastructure.
- **Predictive Insights**: Aggregates data over time to identify emerging trends, supporting data-driven decision making for future system enhancements.
- **Strategic Decision Support**: Offers data-backed insights to guide product evolution.
- **Customizable Alerting System**: Allows defining alerts based on specific metric thresholds, enabling proactive response to potential issues or significant changes in system state.

## Installation

### Prerequisites

1. Create a namespace for the Metrics Operator.
2. Create a secret containing the credentials for the artifactory (read-only) in the operator's namespace.
3. Create a secret containing the data sink credentials in the operator's namespace.

### Deployment

Deploy the Metrics Operator using the Helm chart:

```bash
helm upgrade --install co-metrics-operator deploy-releases-hyperspace-helm/co-metrics-operator \
  --namespace <operator-namespace> \
  --create-namespace \
  --set imagePullSecrets[0].name=<artifactory-secret-name> \
  --version=<version>
```

Replace `<operator-namespace>`, `<artifactory-secret-name>`, and `<version>` with appropriate values.

## Usage

### Metric

Metrics have additional capabilities, such as projections. Projections allow you to extract specific fields from the target resource and include them in the metric data.
This can be useful for tracking additional dimensions of the resource, such as fields, labels or annotations. It uses the dot notation to access nested fields.
The projections are then translated to dimensions in the metric.

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: Metric
metadata:
  name: comp-pod
spec:
  name: comp-metric-pods
  description: Pods
  target:
    kind: Pod
    group: ""
    version: v1
  interval: "1m"
  projections:
    - name: pod-namespace
      fieldPath: "metadata.namespace"
---
```

### Managed Metric

Managed metrics are used to monitor crossplane managed resources. They automatically track resources that have the "crossplane" and "managed" categories in their CRDs.

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: ManagedMetric
metadata:
  name: managed-metric
spec:
  name: managed-metric
  description: Status metric created by an Operator
  kind: Release
  group: helm.crossplane.io
  version: v1beta1
  interval: "1m"
---
```

### Federated Metric
Federated metrics deal with resources that are spread across multiple clusters. To monitor these resources, you need to define a `FederatedMetric` resource.
They offer capabilities to aggregate data as well as filtering down to a specific cluster or field using projections.
```yaml
apiVersion: metrics.cloud.sap/v1alpha1
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
---

```

### Federated Managed Metric
This is a special use case metric, it is looking at all the crossplane managed resource across all clusters.
The pre-condition here is that if a resource comes from a crossplane provider, its CRD should have categories "crossplane" and "managed".


```yaml
apiVersion: metrics.cloud.sap/v1alpha1
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
---
```

## Remote Cluster Access


### Remote Cluster Access

The Metrics Operator can monitor both the cluster it's deployed in and remote clusters. To monitor a remote cluster, define a `RemoteClusterAccess` resource:

This remote cluster access resource can be used by `Metric` and `ManagedMetric` resources to monitor resources in the remote cluster.

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
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


### Federated Cluster Access

To monitor resources across multiple clusters, define a `FederatedClusterAccess` resource:

```yaml
apiVersion: metrics.cloud.sap/v1alpha1
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


## RBAC Configuration

The Metrics Operator requires appropriate permissions to monitor the resources you specify. You need to configure RBAC (Role-Based Access Control) to grant these permissions. Here's an example of how to create a ClusterRole and ClusterRoleBinding for the Metrics Operator:

```yaml
---
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

Replace `<operator-namespace>` with the namespace where the Metrics Operator is deployed. Adjust the `apiGroups` and `resources` fields to match the resources you want to monitor.

Apply the RBAC configuration:

```bash
kubectl apply -f rbac-config.yaml
```

Remember to update this RBAC configuration whenever you add new resource types to monitor.


## Data Sink Integration

The Metrics Operator sends collected data to a configured data sink for storage and analysis. The data sink (e.g., Dynatrace) provides tools for data aggregation, filtering, and visualization.

To make the most of your metrics:

1. Configure your data sink according to its documentation.
2. Use the data sink's query language or UI to create custom views of your metrics.
3. Set up alerts based on metric thresholds or patterns.
4. Leverage the data sink's analysis tools to gain insights into your system's behavior and performance.

For specific instructions on using your data sink's features, refer to its documentation. For example, if using Dynatrace, consult the Dynatrace documentation for information on creating custom charts, setting up alerts, and performing advanced analytics on your metric data.


## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
make dev-local-all
```

2. Run the controller:

```sh
make dev-run
```
Or run it from your IDE.

### Delete Kind Cluster
Delete Kind cluster
```sh
make dev-clean
```

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests generate
```

## Support, Feedback, Contributing

This project is open to feature requests/suggestions, bug reports etc. via [GitHub issues](https://github.com/SAP/metrics-operator/issues). Contribution and feedback are encouraged and always welcome. For more information about how to contribute, the project structure, as well as additional contribution information, see our [Contribution Guidelines](CONTRIBUTING.md).

## Security / Disclosure
If you find any bug that may be a security problem, please follow our instructions at [in our security policy](https://github.com/SAP/metrics-operator/security/policy) on how to report it. Please do not create GitHub issues for security-related doubts or problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/SAP/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright 2024 SAP SE or an SAP affiliate company and metrics-operator contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/SAP/metrics-operator).
