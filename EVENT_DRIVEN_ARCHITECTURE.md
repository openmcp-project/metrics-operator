# Event-Driven Architecture for Metrics Operator

## 1. Current State Analysis

### a. Polling and Orchestrator Usage
- The current controller (`MetricReconciler` in [`internal/controller/metric_controller.go`](internal/controller/metric_controller.go:1)) uses a polling-based reconciliation loop.
- **Polling:** The controller is triggered by changes to `Metric` resources, but actual metric collection is time-based. It checks if enough time has passed since the last observation and schedules the next reconciliation using `RequeueAfter`.
- **Orchestrator:** For each reconciliation, an orchestrator is created with credentials and a query config, responsible for querying the target resources and collecting metric data.

### b. Reconciliation Loop and Timing
- The main logic is in `Reconcile(ctx, req)`:
  - Loads the `Metric` resource.
  - Checks if the interval has elapsed since the last observation.
  - Loads credentials and creates a metric client (for OTEL export).
  - Builds a query config (local or remote cluster).
  - Creates an orchestrator and invokes its `Monitor` method to collect data.
  - Exports metrics via OTEL.
  - Updates the status and schedules the next reconciliation based on the metric's interval or error backoff.

### c. Target Definition and Querying
- **Target Resources:** Defined in the `Metric` spec, but details are abstracted behind the orchestrator and query config.
- The orchestrator is responsible for querying the correct resources, either in the local or a remote cluster, based on the `RemoteClusterAccessRef`.

---

## 2. Event-Driven Architecture Design

### a. Dynamic Informers for Target Resources
- **Dynamic Informers:** Use dynamic informers to watch the resource types specified in each `Metric` spec.
- **Event-Driven:** The controller reacts to create, update, and delete events for the watched resources, triggering metric collection and OTEL export in real-time.
- **Efficiency:** If multiple metrics watch the same resource type, share informers to avoid redundant watches.

### b. Real-Time Event Handling
- On resource events, determine which metrics are interested in the resource and trigger metric updates for those metrics.
- Maintain a mapping from resource types/selectors to the metrics that depend on them.

### c. OTEL Export
- The OTEL export logic remains, but is triggered by resource events rather than by a polling loop.

### d. Efficient Multi-Metric Handling
- Use a central manager to track which metrics are interested in which resource types/selectors.
- Ensure that informers are only created once per resource type/selector combination, and are cleaned up when no longer needed.

---

## 3. Implementation Strategy

### a. Extracting Target Resource Information
- Parse each `Metric` spec to determine:
  - The resource type (GroupVersionKind)
  - Namespace(s) and label selectors
- Maintain a registry of which metrics are interested in which resource types/selectors.

### b. Setting Up Dynamic Informers
- Use the dynamic client and informer factory to create informers for arbitrary resource types at runtime.
- For each unique (GVK, namespace, selector) combination, create (or reuse) an informer.

### c. Managing Informer Lifecycle
- When a new metric is created or updated, add its interest to the registry and ensure the appropriate informer is running.
- When a metric is deleted or changes its target, remove its interest and stop informers that are no longer needed.

### d. Handling Events and Updating Metrics
- On resource events, determine which metrics are affected (using the registry).
- For each affected metric, trigger the metric update and OTEL export.
- Debounce or batch updates if needed to avoid excessive processing.

### e. Backward Compatibility
- Support both polling and event-driven modes during migration.
- Allow metrics to specify whether they use polling or event-driven updates.
- Gradually migrate existing metrics to the new event-driven approach.

---

## 4. Key Components

```mermaid
flowchart TD
    subgraph Operator
        MRC[MetricReconciler (legacy/polling)]
        EDC[EventDrivenController]
        DIM[DynamicInformerManager]
        REH[ResourceEventHandler]
        MUC[MetricUpdateCoordinator]
    end
    subgraph K8s API
        K8s[Resource Events]
    end
    subgraph OTEL
        OTEL[OTEL Exporter]
    end

    MRC --"Polling"--> MUC
    EDC --"Metric Spec"--> DIM
    DIM --"Watches"--> K8s
    K8s --"Events"--> REH
    REH --"Notify"--> MUC
    MUC --"Export"--> OTEL
```

### a. Event-Driven Metric Controller
- Watches `Metric` resources for changes.
- Parses metric specs to determine target resources.
- Registers interest with the Dynamic Informer Manager.

### b. Dynamic Informer Manager
- Manages dynamic informers for arbitrary resource types.
- Ensures informers are shared among metrics with overlapping interests.
- Handles informer lifecycle (start/stop) as metrics are added/removed.

### c. Resource Event Handler
- Receives events from informers.
- Determines which metrics are affected by each event.
- Notifies the Metric Update Coordinator.

### d. Metric Update Coordinator
- Coordinates metric updates and OTEL export.
- Handles batching/debouncing if needed.
- Maintains mapping from resource events to metrics.

---

## 5. Incremental Implementation Plan

1. **Analysis & Registry:** Implement logic to extract target resource info from metric specs and maintain a registry of metric interests.
2. **Dynamic Informers:** Build the Dynamic Informer Manager to create and manage informers for arbitrary resource types.
3. **Event Handling:** Implement the Resource Event Handler to map events to metrics and trigger updates.
4. **Metric Update Coordination:** Refactor metric update/export logic to be callable from both polling and event-driven paths.
5. **Hybrid Mode:** Support both polling and event-driven updates, controlled by a flag in the metric spec.
6. **Migration:** Gradually migrate existing metrics to event-driven mode, monitor performance, and deprecate polling as appropriate.

---

## Summary Table

| Component                  | Responsibility                                                      |
|----------------------------|---------------------------------------------------------------------|
| MetricReconciler           | Legacy polling-based reconciliation                                 |
| EventDrivenController      | Watches Metric CRs, manages event-driven logic                      |
| DynamicInformerManager     | Creates/shares informers for arbitrary resource types               |
| ResourceEventHandler       | Handles resource events, maps to interested metrics                 |
| MetricUpdateCoordinator    | Triggers metric updates and OTEL export, handles batching/debouncing|