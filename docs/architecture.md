# Architecture Overview

The Metrics Operator provides four main resource types for monitoring Kubernetes objects, each suited for different use cases.

## Resource Flows

### Metric

```mermaid
graph LR
    M[Metric] -->|targets via GroupVersionKind| K8S[Kubernetes Objects<br/>Pods, Services, etc.]
    M -.->|optional| RCA[RemoteClusterAccess]
    RCA -->|accesses remote cluster| K8S
    M -->|sends data to| DS[DataSink]

    classDef metricType fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef accessType fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef targetType fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef dataType fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class M metricType
    class RCA accessType
    class K8S targetType
    class DS dataType
```

### ManagedMetric

```mermaid
graph LR
    MM[ManagedMetric] -->|targets managed resources| MR[Managed Resources<br/>with 'crossplane' & 'managed' categories]
    MM -.->|optional| RCA[RemoteClusterAccess]
    RCA -->|accesses remote cluster| MR
    MM -->|sends data to| DS[DataSink]

    classDef metricType fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef accessType fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef targetType fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef dataType fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class MM metricType
    class RCA accessType
    class MR targetType
    class DS dataType
```

### FederatedMetric

```mermaid
graph LR
    FM[FederatedMetric] -->|requires| FCA[FederatedClusterAccess]
    FCA -->|discovers clusters via| CP[ControlPlane Resources]
    FCA -->|provides access to| MC[Multiple Clusters]
    FM -->|targets across clusters| K8S[Kubernetes Objects<br/>across federated clusters]
    FM -->|aggregates & sends to| DS[DataSink]

    classDef metricType fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef accessType fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef targetType fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef dataType fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class FM metricType
    class FCA accessType
    class CP,MC,K8S targetType
    class DS dataType
```

### FederatedManagedMetric

```mermaid
graph LR
    FMM[FederatedManagedMetric] -->|requires| FCA[FederatedClusterAccess]
    FCA -->|discovers clusters via| CP[ControlPlane Resources]
    FCA -->|provides access to| MC[Multiple Clusters]
    FMM -->|targets managed resources<br/>across clusters| MR[Managed Resources<br/>with 'crossplane' & 'managed' categories]
    FMM -->|aggregates & sends to| DS[DataSink]

    classDef metricType fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef accessType fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef targetType fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef dataType fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class FMM metricType
    class FCA accessType
    class CP,MC,MR targetType
    class DS dataType
```

## Resource Types

| Resource | CRD | Description |
|---|---|---|
| **Metric** | [metrics.openmcp.cloud_metrics.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_metrics.yaml) | Monitors specific Kubernetes resources in local or remote clusters using GroupVersionKind targeting |
| **ManagedMetric** | [metrics.openmcp.cloud_managedmetrics.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_managedmetrics.yaml) | Specialized for monitoring Crossplane managed resources (categories: "crossplane" + "managed") |
| **FederatedMetric** | [metrics.openmcp.cloud_federatedmetrics.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_federatedmetrics.yaml) | Monitors resources across multiple clusters, aggregating data from federated sources |
| **FederatedManagedMetric** | [metrics.openmcp.cloud_federatedmanagedmetrics.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_federatedmanagedmetrics.yaml) | Monitors Crossplane managed resources across multiple clusters |
| **RemoteClusterAccess** | [metrics.openmcp.cloud_remoteclusteraccesses.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_remoteclusteraccesses.yaml) | Access configuration for monitoring resources in remote clusters |
| **FederatedClusterAccess** | [metrics.openmcp.cloud_federatedclusteraccesses.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_federatedclusteraccesses.yaml) | Discovers and provides access to multiple clusters for federated monitoring |
| **DataSink** | [metrics.openmcp.cloud_datasinks.yaml](../cmd/metrics-operator/embedded/crds/metrics.openmcp.cloud_datasinks.yaml) | Defines where metrics data should be sent (Dynatrace, custom OTLP endpoints) |

## Data Flow

All metrics are exported via [OpenTelemetry](https://opentelemetry.io) protocol (OTLP) to the configured DataSink. The operator collects resource counts and custom dimensions at each configured interval, then pushes gauge metrics to the OTLP endpoint.
