#!/bin/bash

echo "=== Debug Test for Event-Driven Metrics ==="

# Function to check if kubectl command exists
check_kubectl() {
    if ! command -v kubectl &> /dev/null; then
        echo "kubectl is not installed or not in PATH"
        exit 1
    fi
}

# Function to wait for metric to be ready
wait_for_metric() {
    echo "Waiting for metric to be processed..."
    for i in {1..30}; do
        if kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}' 2>/dev/null | grep -q "T"; then
            echo "Metric has observation timestamp"
            break
        fi
        echo "Waiting... ($i/30)"
        sleep 2
    done
}

check_kubectl

echo "1. Applying test metric..."
kubectl apply -f test-metric.yaml

wait_for_metric

echo "2. Current metric status:"
kubectl get metric test-pods -o yaml | grep -A 10 "status:"

echo "3. Current pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo "4. Creating test pod..."
kubectl run debug-test-pod --image=nginx --restart=Never

echo "5. Waiting 15 seconds for events to be processed..."
sleep 15

echo "6. Updated metric status:"
kubectl get metric test-pods -o yaml | grep -A 10 "status:"

echo "7. Updated pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo "8. Cleaning up test pod..."
kubectl delete pod debug-test-pod --ignore-not-found=true

echo "9. Waiting 15 seconds for deletion event..."
sleep 15

echo "10. Final metric status:"
kubectl get metric test-pods -o yaml | grep -A 10 "status:"

echo "11. Final pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo ""
echo "=== Debug Test Complete ==="
echo ""
echo "To check operator logs, run:"
echo "kubectl logs -l app.kubernetes.io/name=metrics-operator -n metrics-operator-system --tail=100"
echo ""
echo "Look for these log messages:"
echo "- 'Starting informer for target' (should show Pod informer being created)"
echo "- 'DynamicInformer Event: Add' (should show Pod events being received)"
echo "- 'OnAdd event received' (should show ResourceEventHandler receiving events)"
echo "- 'Handling event' (should show event processing)"
echo "- 'Metric is interested in this event' (should show metric matching)"
echo "- 'MetricUpdateCoordinator: Metric update requested' (should show update requests)"
