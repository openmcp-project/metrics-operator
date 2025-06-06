#!/bin/bash

echo "=== Checking Operator Logs for Event Processing ==="

# Check if the operator is running in the current namespace (development mode)
if kubectl get pods | grep -q "metrics-operator\|main"; then
    echo "Found operator running locally, checking logs..."
    # For local development, logs might be in stdout
    echo "Please check the terminal where 'make run' is running for these log messages:"
else
    # Check if operator is running in a system namespace
    OPERATOR_NAMESPACE=""
    for ns in "metrics-operator-system" "default" "kube-system"; do
        if kubectl get pods -n $ns -l app.kubernetes.io/name=metrics-operator 2>/dev/null | grep -q "metrics-operator"; then
            OPERATOR_NAMESPACE=$ns
            break
        fi
    done
    
    if [ -n "$OPERATOR_NAMESPACE" ]; then
        echo "Found operator in namespace: $OPERATOR_NAMESPACE"
        echo "Checking recent logs..."
        kubectl logs -n $OPERATOR_NAMESPACE -l app.kubernetes.io/name=metrics-operator --tail=50
    else
        echo "Could not find metrics-operator pods. Please check manually with:"
        echo "kubectl get pods --all-namespaces | grep metrics-operator"
    fi
fi

echo ""
echo "=== Key Log Messages to Look For ==="
echo "1. 'Starting informer for target' - Shows Pod informer being created"
echo "2. 'DynamicInformer Event: Add' - Shows Pod events being received"
echo "3. 'OnAdd event received' - Shows ResourceEventHandler receiving events"
echo "4. 'Handling event' - Shows event processing"
echo "5. 'Metric is interested in this event' - Shows metric matching"
echo "6. 'MetricUpdateCoordinator: Metric update requested' - Shows update requests"
echo ""
echo "If running locally with 'make run', check the terminal output for these messages."
