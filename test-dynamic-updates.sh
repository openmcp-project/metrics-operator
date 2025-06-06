#!/bin/bash

echo "Testing dynamic metric updates..."

# Apply the test metric
echo "1. Applying test metric..."
kubectl apply -f test-metric.yaml

# Wait a moment for the metric to be processed
echo "2. Waiting for metric to be registered..."
sleep 5

# Check initial metric status
echo "3. Initial metric status:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

# Create a test pod to trigger an event
echo "4. Creating a test pod to trigger metric update..."
kubectl run test-pod-1 --image=nginx --restart=Never

# Wait for the event to be processed
echo "5. Waiting for metric update..."
sleep 10

# Check updated metric status
echo "6. Updated metric status after adding pod:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

# Create another test pod
echo "7. Creating another test pod..."
kubectl run test-pod-2 --image=nginx --restart=Never

# Wait for the event to be processed
echo "8. Waiting for metric update..."
sleep 10

# Check updated metric status again
echo "9. Updated metric status after adding second pod:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

# Clean up
echo "10. Cleaning up test pods..."
kubectl delete pod test-pod-1 test-pod-2 --ignore-not-found=true

# Wait for deletion events to be processed
echo "11. Waiting for deletion events..."
sleep 10

# Check final metric status
echo "12. Final metric status after pod deletion:"
kubectl get metric test-pods -o jsonpath='{.status.observation.latestValue}' && echo

echo "Test completed!"
