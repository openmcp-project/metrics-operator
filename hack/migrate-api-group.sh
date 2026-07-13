#!/usr/bin/env bash
# One-time migration: metrics.openmcp.cloud/v1alpha1 → metrics.open-control-plane.io/v1alpha1
#
# For each resource kind this script:
#   1. Dumps all existing objects (old group)
#   2. Re-applies them under the new group (new group CRDs must already be installed)
#   3. Deletes the old objects
#
# Prerequisites:
#   - New CRDs (metrics.open-control-plane.io) already applied to the cluster
#   - Old CRDs (metrics.openmcp.cloud) still present (will be removed last)
#   - kubectl configured to the target cluster
#   - yq >= v4 in PATH
#
# Usage:
#   ./hack/migrate-api-group.sh [--dry-run]

set -euo pipefail

OLD_GROUP="metrics.openmcp.cloud"
NEW_GROUP="metrics.open-control-plane.io"
VERSION="v1alpha1"

KINDS=(
  datasinks
  federatedclusteraccesses
  federatedmanagedmetrics
  federatedmetrics
  managedmetrics
  metrics
  remoteclusteraccesses
)

DRY_RUN="${1:-}"
KUBECTL_FLAGS=""
if [[ "${DRY_RUN}" == "--dry-run" ]]; then
  KUBECTL_FLAGS="--dry-run=client"
  echo ">>> DRY RUN mode — no changes will be persisted"
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

echo ">>> Fetching all namespaces"
NAMESPACES=$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}')

for KIND in "${KINDS[@]}"; do
  echo ""
  echo "=== Migrating: ${KIND} ==="

  for NS in ${NAMESPACES}; do
    ITEMS=$(kubectl get "${KIND}.${OLD_GROUP}" -n "${NS}" --ignore-not-found -o json 2>/dev/null)
    COUNT=$(echo "${ITEMS}" | python3 -c "import sys,json; print(len(json.load(sys.stdin).get('items',[])))" 2>/dev/null || echo 0)

    if [[ "${COUNT}" == "0" ]]; then
      continue
    fi

    echo "  namespace=${NS}: found ${COUNT} object(s)"

    # Write each object rewritten to new apiVersion
    echo "${ITEMS}" | python3 - <<'PYEOF' "${NS}" "${KIND}" "${OLD_GROUP}" "${NEW_GROUP}" "${VERSION}" "${TMPDIR}" "${KUBECTL_FLAGS}"
import sys, json, subprocess, os, tempfile

ns, kind, old_group, new_group, version, tmpdir, kubectl_flags = sys.argv[1:]
data = json.load(sys.stdin)

for item in data.get("items", []):
    name = item["metadata"]["name"]
    # Rewrite apiVersion; strip managed fields and resourceVersion so apply is clean
    item["apiVersion"] = f"{new_group}/{version}"
    item["metadata"].pop("resourceVersion", None)
    item["metadata"].pop("uid", None)
    item["metadata"].pop("creationTimestamp", None)
    item["metadata"].pop("generation", None)
    item["metadata"].pop("managedFields", None)
    item.get("status", {}).clear()  # status is managed by controller

    manifest = json.dumps(item)
    fname = os.path.join(tmpdir, f"{kind}-{ns}-{name}.json")
    with open(fname, "w") as f:
        f.write(manifest)

    print(f"    apply: {kind}/{name} in {ns}")
    cmd = ["kubectl", "apply", "-f", fname]
    if kubectl_flags:
        cmd.append(kubectl_flags)
    subprocess.run(cmd, check=True)

    print(f"    delete old: {kind}.{old_group}/{name} in {ns}")
    del_cmd = ["kubectl", "delete", f"{kind}.{old_group}", name, "-n", ns]
    if kubectl_flags:
        del_cmd.append(kubectl_flags)
    subprocess.run(del_cmd, check=True)
PYEOF

  done
done

echo ""
echo ">>> Migration complete."
echo ">>> You can now remove the old CRDs:"
for KIND in "${KINDS[@]}"; do
  echo "    kubectl delete crd ${KIND}.${OLD_GROUP}"
done
