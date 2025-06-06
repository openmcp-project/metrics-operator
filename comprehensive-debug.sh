#!/bin/bash

echo "=== Comprehensive Debug Test ==="

# Function to get current timestamp
timestamp() {
    date '+%Y-%m-%d %H:%M:%S'
}

echo "$(timestamp) - Starting comprehensive debug test"

echo "$(timestamp) - 1. Applying test metric..."
kubectl apply -f test-metric.yaml

echo "$(timestamp) - 2. Waiting for metric to be processed..."
sleep 5

echo "$(timestamp) - 3. Initial state:"
echo "  Metric value: $(kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}')"
echo "  Pod count: $(kubectl get pods -n default --no-headers | wc -l)"

echo "$(timestamp) - 4. Creating test pod..."
kubectl run comprehensive-test-pod --image=nginx --restart=Never -n default

echo "$(timestamp) - 5. Waiting 10 seconds for add event..."
sleep 10

echo "$(timestamp) - 6. After pod creation:"
echo "  Metric value: $(kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}')"
echo "  Pod count: $(kubectl get pods -n default --no-headers | wc -l)"

# Store the timestamp after add event
ADD_TIMESTAMP=$(kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}')

echo "$(timestamp) - 7. Deleting test pod..."
kubectl delete pod comprehensive-test-pod -n default

echo "$(timestamp) - 8. Waiting 15 seconds for delete event..."
sleep 15

echo "$(timestamp) - 9. After pod deletion:"
echo "  Metric value: $(kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}')"
echo "  Pod count: $(kubectl get pods -n default --no-headers | wc -l)"

# Store the timestamp after delete event
DELETE_TIMESTAMP=$(kubectl get metric test-pods -o jsonpath='{.status.observation.timestamp}')

echo "$(timestamp) - 10. Analysis:"
if [ "$ADD_TIMESTAMP" = "$DELETE_TIMESTAMP" ]; then
    echo "  ❌ ISSUE: Metric timestamp did NOT change after delete event"
    echo "     This means the metric was NOT recalculated after pod deletion"
    echo "     The delete event was either not received or not processed"
else
    echo "  ✅ Metric timestamp changed after delete event"
    echo "     This means the metric was recalculated, but value might be wrong"
fi

echo "$(timestamp) - 11. Full metric status:"
kubectl get metric test-pods -o yaml | grep -A 15 "status:"

echo ""
echo "=== Debug Summary ==="
echo "Add timestamp:    $ADD_TIMESTAMP"
echo "Delete timestamp: $DELETE_TIMESTAMP"
echo ""
echo "Next steps:"
echo "1. Check operator logs for delete event messages"
echo "2. If no delete events in logs: Issue is in informer setup"
echo "3. If delete events in logs but no timestamp change: Issue is in event processing"
echo "4. If timestamp changes but value wrong: Issue is in metric calculation"
