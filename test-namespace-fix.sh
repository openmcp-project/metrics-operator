#!/bin/bash

echo "=== Testing Namespace-Specific Informer Fix ==="

echo "1. Applying test metric..."
kubectl apply -f test-metric.yaml

echo "2. Waiting for metric to be processed..."
sleep 5

echo "3. Current metric status:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "4. Current pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo "5. Creating test pod in default namespace..."
kubectl run test-fix-pod --image=nginx --restart=Never -n default

echo "6. Waiting 10 seconds for event processing..."
sleep 10

echo "7. Updated metric status (should change from previous value):"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "8. Updated pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo "9. Creating pod in different namespace (should NOT affect metric)..."
kubectl create namespace test-ns 2>/dev/null || true
kubectl run test-other-pod --image=nginx --restart=Never -n test-ns

echo "10. Waiting 10 seconds..."
sleep 10

echo "11. Metric status (should NOT change from step 7):"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "12. Pod count in default namespace (should be same as step 8):"
kubectl get pods -n default --no-headers | wc -l

echo "13. Pod count in test-ns namespace:"
kubectl get pods -n test-ns --no-headers | wc -l

echo "14. Cleaning up..."
kubectl delete pod test-fix-pod -n default --ignore-not-found=true
kubectl delete pod test-other-pod -n test-ns --ignore-not-found=true
kubectl delete namespace test-ns --ignore-not-found=true

echo "15. Waiting 10 seconds for cleanup..."
sleep 10

echo "16. Final metric status:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "17. Final pod count in default namespace:"
kubectl get pods -n default --no-headers | wc -l

echo ""
echo "=== Test Complete ==="
echo ""
echo "Expected behavior:"
echo "- Step 7 should show metric value increased by 1 from step 3"
echo "- Step 11 should show same value as step 7 (other namespace pod ignored)"
echo "- Step 16 should show value decreased by 1 from step 11"
echo ""
echo "If the values change as expected, the namespace-specific informer fix is working!"
