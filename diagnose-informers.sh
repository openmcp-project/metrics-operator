#!/bin/bash

echo "=== Diagnosing Dynamic Informer Setup ==="

echo "1. Checking if test metric exists and is registered..."
kubectl get metric test-pods -o yaml | grep -A 20 "spec:"

echo ""
echo "2. Checking metric status..."
kubectl get metric test-pods -o jsonpath='{.status}' | jq '.' 2>/dev/null || kubectl get metric test-pods -o yaml | grep -A 10 "status:"

echo ""
echo "3. Testing if operator is receiving metric events..."
echo "   Updating metric description to trigger reconciliation..."
kubectl patch metric test-pods --type='merge' -p='{"spec":{"description":"Updated description to trigger reconciliation"}}'

echo ""
echo "4. Waiting 10 seconds for reconciliation..."
sleep 10

echo ""
echo "5. Checking if metric was reconciled..."
kubectl get metric test-pods -o yaml | grep "Updated description"

echo ""
echo "=== Diagnosis Complete ==="
echo ""
echo "Now run './check-logs.sh' or check the operator terminal output for:"
echo "- 'Reconciling Metric for event-driven setup'"
echo "- 'Starting informer for target'"
echo "- Any error messages"
