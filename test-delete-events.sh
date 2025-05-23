#!/bin/bash

echo "=== Testing Delete Event Processing ==="

echo "1. Applying test metric..."
kubectl apply -f test-metric.yaml

echo "2. Waiting for metric to be processed..."
sleep 5

echo "3. Current metric status:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "4. Creating test pod..."
kubectl run delete-test-pod --image=nginx --restart=Never -n default

echo "5. Waiting 10 seconds for add event..."
sleep 10

echo "6. Metric status after pod creation:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "7. Pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo "8. Deleting test pod..."
kubectl delete pod delete-test-pod -n default

echo "9. Waiting 15 seconds for delete event processing..."
sleep 15

echo "10. Metric status after pod deletion (should decrease):"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "11. Pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo ""
echo "=== Test Complete ==="
echo ""
echo "Check operator logs for these messages:"
echo "- 'DynamicInformer Event: Delete' (from DynamicInformerManager)"
echo "- 'OnDelete event received' (from ResourceEventHandler)"
echo "- 'Handling event' with eventType=delete"
echo "- 'Metric is interested in this event' for delete events"
echo "- 'MetricUpdateCoordinator: Metric update requested' for delete events"
echo ""
echo "If delete events are not appearing in logs, the issue is in the informer setup."
echo "If delete events appear but metric doesn't update, the issue is in metric processing."
