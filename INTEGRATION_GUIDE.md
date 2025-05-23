# Event-Driven Architecture Integration Guide

This guide explains how to integrate the new event-driven architecture components into the metrics operator.

## Components Created

1. **TargetRegistry** (`internal/controller/targetregistry.go`)
   - Tracks which Metric CRs are interested in which Kubernetes resources
   - Maps GVK + namespace + selector to interested metrics

2. **DynamicInformerManager** (`internal/controller/dynamicinformermanager.go`)
   - Manages dynamic informers for arbitrary Kubernetes resource types
   - Shares informers efficiently across multiple metrics
   - Handles informer lifecycle (start/stop)

3. **ResourceEventHandler** (`internal/controller/resourceeventhandler.go`)
   - Handles events from dynamic informers
   - Maps resource events to interested Metric CRs
   - Triggers metric updates via the coordinator

4. **MetricUpdateCoordinator** (`internal/controller/metricupdatecoordinator.go`)
   - Coordinates metric updates and OTEL export
   - Contains refactored logic from the original MetricReconciler
   - Can be called from both event-driven and polling paths

5. **EventDrivenController** (`internal/controller/eventdrivencontroller.go`)
   - Main controller that ties all components together
   - Watches Metric CRs and manages the dynamic informer setup
   - Coordinates the event-driven system lifecycle

## Integration Steps

### 1. Update main.go

Add the EventDrivenController to your main controller manager setup:

```go
// In cmd/main.go or wherever you set up controllers

import (
    "github.com/SAP/metrics-operator/internal/controller"
)

func main() {
    // ... existing setup ...
    
    // Set up the existing MetricReconciler (for backward compatibility)
    if err = (&controller.MetricReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "Metric")
        os.Exit(1)
    }
    
    // Set up the new EventDrivenController
    eventDrivenController := controller.NewEventDrivenController(mgr)
    if err = eventDrivenController.SetupWithManager(mgr); err != nil {
        setupLog.Error(err, "unable to create controller", "controller", "EventDriven")
        os.Exit(1)
    }
    
    // Start the event-driven system after the manager starts
    go func() {
        <-mgr.Elected() // Wait for leader election if enabled
        ctx := ctrl.SetupSignalHandler()
        if err := eventDrivenController.Start(ctx); err != nil {
            setupLog.Error(err, "failed to start event-driven controller")
        }
    }()
    
    // ... rest of setup ...
}
```

### 2. Hybrid Mode Implementation

To support both polling and event-driven modes, you can:

#### Option A: Add a field to MetricSpec
```go
// In api/v1alpha1/metric_types.go
type MetricSpec struct {
    // ... existing fields ...
    
    // EventDriven enables real-time event-driven metric collection
    // +optional
    EventDriven *bool `json:"eventDriven,omitempty"`
}
```

#### Option B: Use annotations
```yaml
apiVersion: metrics.cloud.sap/v1alpha1
kind: Metric
metadata:
  name: my-metric
  annotations:
    metrics.cloud.sap/event-driven: "true"
spec:
  # ... metric spec ...
```

### 3. Update Existing MetricReconciler

Modify the existing MetricReconciler to use the MetricUpdateCoordinator:

```go
// In internal/controller/metric_controller.go

func (r *MetricReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... existing setup ...
    
    // Check if this metric should use event-driven updates
    if isEventDriven(&metric) {
        // Skip polling-based reconciliation for event-driven metrics
        // The EventDrivenController will handle updates
        return ctrl.Result{}, nil
    }
    
    // For polling-based metrics, use the MetricUpdateCoordinator
    coordinator := NewMetricUpdateCoordinator(
        r.getClient(),
        r.log,
        r.getRestConfig(),
        r.Recorder,
        r.Scheme,
    )
    
    if err := coordinator.processMetric(ctx, &metric, r.log); err != nil {
        return ctrl.Result{RequeueAfter: RequeueAfterError}, err
    }
    
    // Schedule next reconciliation based on interval
    return r.scheduleNextReconciliation(&metric)
}

func isEventDriven(metric *v1alpha1.Metric) bool {
    // Check annotation or spec field
    if metric.Annotations["metrics.cloud.sap/event-driven"] == "true" {
        return true
    }
    if metric.Spec.EventDriven != nil && *metric.Spec.EventDriven {
        return true
    }
    return false
}
```

## Benefits

1. **Real-time Updates**: Metrics are updated immediately when target resources change
2. **Reduced API Load**: No more polling every interval for all metrics
3. **Efficient Resource Usage**: Shared informers across multiple metrics
4. **Backward Compatibility**: Existing polling-based metrics continue to work
5. **Incremental Migration**: Can gradually migrate metrics to event-driven mode

## Testing

1. Create a test Metric CR with event-driven enabled
2. Create/update/delete target resources
3. Verify metrics are updated in real-time
4. Check OTEL exports are triggered by events
5. Verify informers are cleaned up when metrics are deleted

## Monitoring

The event-driven system provides several logging points:

- EventDrivenController: Metric registration and informer management
- DynamicInformerManager: Informer lifecycle events
- ResourceEventHandler: Resource event processing
- MetricUpdateCoordinator: Metric processing and export

Use these logs to monitor the health and performance of the event-driven system.