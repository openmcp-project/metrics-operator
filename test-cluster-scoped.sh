#!/bin/bash

echo "=== Testing Cluster-Scoped Resource Events ==="

# Function to get current timestamp
timestamp() {
    date '+%Y-%m-%d %H:%M:%S'
}

echo "$(timestamp) - Starting cluster-scoped resource test"

echo "$(timestamp) - 1. Applying cluster-scoped metric (Namespace count)..."
kubectl apply -f test-cluster-metric.yaml

echo "$(timestamp) - 2. Waiting for metric to be processed..."
sleep 5

echo "$(timestamp) - 3. Initial state:"
echo "  Metric value: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.timestamp}')"
echo "  Namespace count: $(kubectl get namespaces --no-headers | wc -l)"

echo "$(timestamp) - 4. Listing current namespaces:"
kubectl get namespaces --no-headers | awk '{print "  - " $1}'

echo "$(timestamp) - 5. Creating test namespace..."
kubectl create namespace test-cluster-scope-ns

echo "$(timestamp) - 6. Waiting 10 seconds for add event..."
sleep 10

echo "$(timestamp) - 7. After namespace creation:"
echo "  Metric value: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.timestamp}')"
echo "  Namespace count: $(kubectl get namespaces --no-headers | wc -l)"

# Store the timestamp after add event
ADD_TIMESTAMP=$(kubectl get metric test-namespaces -o jsonpath='{.status.observation.timestamp}')

echo "$(timestamp) - 8. Deleting test namespace..."
kubectl delete namespace test-cluster-scope-ns

echo "$(timestamp) - 9. Waiting 15 seconds for delete event..."
sleep 15

echo "$(timestamp) - 10. After namespace deletion:"
echo "  Metric value: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.latestValue}')"
echo "  Metric timestamp: $(kubectl get metric test-namespaces -o jsonpath='{.status.observation.timestamp}')"
echo "  Namespace count: $(kubectl get namespaces --no-headers | wc -l)"

# Store the timestamp after delete event
DELETE_TIMESTAMP=$(kubectl get metric test-namespaces -o jsonpath='{.status.observation.timestamp}')

echo "$(timestamp) - 11. Analysis:"
if [ "$ADD_TIMESTAMP" = "$DELETE_TIMESTAMP" ]; then
    echo "  ❌ ISSUE: Metric timestamp did NOT change after delete event"
    echo "     This means cluster-scoped delete events are not being processed"
else
    echo "  ✅ Metric timestamp changed after delete event"
    echo "     This means cluster-scoped events are being processed correctly"
fi

echo "$(timestamp) - 12. Full metric status:"
kubectl get metric test-namespaces -o yaml | grep -A 15 "status:"

echo ""
echo "=== Cluster-Scoped Test Summary ==="
echo "Add timestamp:    $ADD_TIMESTAMP"
echo "Delete timestamp: $DELETE_TIMESTAMP"
echo ""
echo "Expected behavior for cluster-scoped resources:"
echo "- Metric should increase when namespace is created"
echo "- Metric should decrease when namespace is deleted"
echo "- Timestamps should change for both events"
echo ""
echo "Check operator logs for:"
echo "- 'Starting informer for target' with Kind=Namespace"
echo "- 'DynamicInformer Event: Add/Delete' for Namespace events"
echo "- 'OnAdd/OnDelete event received' with gvk containing Namespace"
echo "- 'Handling event' with eventType=add/delete for Namespaces"
echo ""
echo "Cleanup:"
echo "kubectl delete -f test-cluster-metric.yaml"
