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

To create a new metric, deploy a `Metric` resource in your desired namespace. The Metrics Operator will pick up the resource and start monitoring it, periodically sending data points to the configured data sink.

Example `Metric` resource:

```yaml
apiVersion: insight.orchestrate.cloud.sap/v1alpha1
kind: Metric
metadata:
    name: example-metric
    namespace: <metric-namespace>
spec:
    description: Description of the metric
    frequency: 1
    group: example.group
    kind: ExampleResource
    name: example-metric-name
    version: v1
```

Apply the metric:

```bash
kubectl apply -f metric.yaml
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

## Remote Cluster Access

The Metrics Operator can monitor both the cluster it's deployed in and remote clusters. To monitor a remote cluster, define a `RemoteClusterAccess` resource:

```yaml
apiVersion: insight.orchestrate.cloud.sap/v1alpha1
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
