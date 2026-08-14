[![REUSE status](https://api.reuse.software/badge/github.com/openmcp-project/metrics-operator)](https://api.reuse.software/info/github.com/openmcp-project/metrics-operator)

# Metrics Operator

A Kubernetes operator that collects counts and attributes of cluster resources and exports them as [OpenTelemetry](https://opentelemetry.io) gauge metrics to configurable data sinks (e.g., Dynatrace).

## Key Features

- **Flexible targeting**: Monitor any Kubernetes or Crossplane resource by GroupVersionKind
- **Federation**: Aggregate metrics across multiple clusters
- **Rich dimensions**: Extract labels, annotations, conditions, or any field as metric attributes
- **Comprehensive Resource Tracking**: Quantifies and catalogs various resource types, providing a holistic view of resource distribution and utilization.
- **Multi-dimensional Analysis**: Examines specific attributes and dimensions of resources, generating nuanced metrics for deeper understanding of system behavior.
- **Comparative Analytics**: Enables side-by-side analysis of different resource configurations, highlighting patterns and potential imbalances in resource allocation.
- **Custom Component Focus**: Tailored to monitor and analyze complex, custom-defined resources across your infrastructure.
- **Predictive Insights**: Aggregates data over time to identify emerging trends, supporting data-driven decision making for future system enhancements.
- **Strategic Decision Support**: Offers data-backed insights to guide product evolution.
- **Customizable Alerting System**: Allows defining alerts based on specific metric thresholds, enabling proactive response to potential issues or significant changes in system state.
- **Pluggable sinks**: Send to any OTLP-compatible endpoint via `DataSink` CRDs
- **Standardized**: Full OpenTelemetry protocol support
- **ServiceMonitor integration**: Create `ServiceMonitor` resources for Prometheus scraping

#### Current Limitations
- Pod-level log aggregation & collection not currently supported
- Pod metrics collection feature in [backlog](https://github.com/openmcp-project/metrics-operator/issues/70)
- 
## Documentation

| Topic                                                        | Description                                                             |
| ------------------------------------------------------------ | ----------------------------------------------------------------------- |
| [Installation](docs/installation.md)                         | Helm deployment and post-install setup                                  |
| [Architecture](docs/architecture.md)                         | Resource types, flows, and data model                                   |
| [Usage](docs/usage.md)                                       | Metric, ManagedMetric, FederatedMetric, FederatedManagedMetric examples |
| [Dimensions Configuration](docs/dimensions-configuration.md) | Projections, valueFrom, cardinality                                     |
| [Metrics Export](docs/metrics-export.md)                     | DataSink (OTLP push), ServiceMonitor (Prometheus scrape)                |
| [Remote Cluster Access](docs/remote-cluster-access.md)       | RemoteClusterAccess and FederatedClusterAccess                          |
| [RBAC](docs/rbac.md)                                         | Permissions required for monitored resources                            |
| [Development](docs/development.md)                           | Local development with kind                                             |

## Quickstart

**Via Open Control Plane** (name must match your `ControlPlane` resource):

```yaml
apiVersion: metrics.services.open-control-plane.io/v1alpha1
kind: MetricsOperator
metadata:
  name: my-mcp
  namespace: <your-ocp-namespace>
spec:
  version: "v0.13.0"    # pick latest version from GitHub Releases
```

**OR Via Helm:**

```bash
helm upgrade --install metrics-operator oci://ghcr.io/openmcp-project/charts/metrics-operator \
  --namespace metrics-operator-system \
  --create-namespace \
  --version=<version>
```

See [Installation](docs/installation.md) for full setup instructions including DataSink configuration.

## Support, Feedback, Contributing

Feature requests, bug reports, and contributions are welcome via [GitHub issues](https://github.com/openmcp-project/metrics-operator/issues). See our [Contribution Guidelines](https://github.com/openmcp-project/.github/blob/main/CONTRIBUTING.md).

## Security / Disclosure

Report security issues following our [security policy](https://github.com/openmcp-project/metrics-operator/security/policy). Do not open public GitHub issues for security problems.

## Code of Conduct

We as members, contributors, and leaders pledge to make participation in our community a harassment-free experience for everyone. By participating in this project, you agree to abide by its [Code of Conduct](https://github.com/openmcp-project/.github/blob/main/CODE_OF_CONDUCT.md) at all times.

## Licensing

Copyright OpenControlPlane contributors. Please see our [LICENSE](LICENSE) for copyright and license information. Detailed information including third-party components and their licensing/copyright information is available [via the REUSE tool](https://api.reuse.software/info/github.com/openmcp-project/metrics-operator).

---

<p align="center">
  <a href="https://apeirora.eu/content/projects/">
    <img alt="BMWK-EU funding logo" src="https://apeirora.eu/assets/img/BMWK-EU.png" width="300"/>
  </a>
</p>

<p align="center">
  OpenControlPlane is part of <a href="https://apeirora.eu/content/projects/">ApeiroRA</a>, an EU Important Project of Common European Interest (IPCEI-CIS).
</p>

<p align="center">
  Copyright Linux Foundation Europe. For web site terms of use, trademark policy and other project policies please see <a href="https://linuxfoundation.eu/en/policies">https://linuxfoundation.eu/en/policies</a>.
</p>
