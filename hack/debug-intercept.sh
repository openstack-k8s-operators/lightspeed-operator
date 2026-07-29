#!/bin/bash
# Override the lightspeed-service-api container to sleep so you can rsh in and
# run the service manually.  Scales down the operator first so it doesn't
# revert the patch.
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config-crc}"
NS="${LIGHTSPEED_NS:-openstack-lightspeed}"
DEPLOYMENT="${DEPLOYMENT:-lightspeed-stack-deployment}"
CONTAINER="lightspeed-service-api"
OPERATOR_DEPLOY="openstack-lightspeed-operator-controller-manager"

echo "==> Scaling down operator in ${NS}..."
oc scale deployment "$OPERATOR_DEPLOY" -n "$NS" --replicas=0

echo "==> Patching ${DEPLOYMENT} to sleep ${CONTAINER}..."
# Find the container index dynamically
IDX=$(oc get deployment "$DEPLOYMENT" -n "$NS" \
  -o jsonpath='{range .spec.template.spec.containers[*]}{.name}{"\n"}{end}' \
  | grep -n "^${CONTAINER}$" | cut -d: -f1)
IDX=$((IDX - 1))  # 0-based

oc patch deployment "$DEPLOYMENT" -n "$NS" --type json -p "[
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/${IDX}/command\", \"value\": [\"sh\"]},
  {\"op\": \"replace\", \"path\": \"/spec/template/spec/containers/${IDX}/args\", \"value\": [\"-c\", \"sleep infinity\"]},
  {\"op\": \"remove\", \"path\": \"/spec/template/spec/containers/${IDX}/readinessProbe\"},
  {\"op\": \"remove\", \"path\": \"/spec/template/spec/containers/${IDX}/livenessProbe\"},
  {\"op\": \"remove\", \"path\": \"/spec/template/spec/containers/${IDX}/startupProbe\"}
]"

echo "==> Waiting for rollout..."
oc rollout status "deployment/${DEPLOYMENT}" -n "$NS" --timeout=120s

POD=$(oc get pods -n "$NS" -l app.kubernetes.io/name=openstack-lightspeed-app-server \
  --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')

echo ""
echo "Pod ready: ${POD}"
echo ""
echo "To connect:"
echo "  oc rsh -n ${NS} -c ${CONTAINER} ${POD}"
echo ""
echo "Then run the service with:"
echo "  lightspeed-stack -c /vector-db-discovered-values/lightspeed-stack.yaml -v"
echo ""
echo "Or sync local code before connecting and then start the service inside the container:"
echo "  ./scripts/debug-sync.sh"
echo "  PYTHONPATH=/tmp/src lightspeed-stack -c /vector-db-discovered-values/lightspeed-stack.yaml -v"
