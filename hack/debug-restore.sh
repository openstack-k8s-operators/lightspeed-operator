#!/bin/bash
# Undo the debug-intercept patch and scale the operator back up.
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config-crc}"
NS="${LIGHTSPEED_NS:-openstack-lightspeed}"
DEPLOYMENT="${DEPLOYMENT:-lightspeed-stack-deployment}"
OPERATOR_DEPLOY="openstack-lightspeed-operator-controller-manager"

echo "==> Rolling back ${DEPLOYMENT}..."
oc rollout undo "deployment/${DEPLOYMENT}" -n "$NS"
oc rollout status "deployment/${DEPLOYMENT}" -n "$NS" --timeout=120s

echo "==> Scaling up operator in ${NS}..."
oc scale deployment "$OPERATOR_DEPLOY" -n "$NS" --replicas=1

echo ""
echo "Restored. The operator will reconcile the deployment."
