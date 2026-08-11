#!/bin/bash
# Sync local src/ into the running lightspeed-service-api container,
# placing it in /tmp/src so PYTHONPATH can override the installed package.
# Uses tar + python tarfile since the container has no tar/rsync binaries.
set -euo pipefail

export KUBECONFIG="${KUBECONFIG:-$HOME/.kube/config-crc}"
NS="${LIGHTSPEED_NS:-openstack-lightspeed}"
CONTAINER="lightspeed-service-api"
SRC_DIR="${1:-./src}"

POD=$(oc get pods -n "$NS" -l app.kubernetes.io/name=openstack-lightspeed-app-server \
  --field-selector=status.phase=Running -o jsonpath='{.items[0].metadata.name}')

echo "==> Syncing ${SRC_DIR}/ -> ${POD}:/tmp/src/"
tar cf - -C "${SRC_DIR}" . | oc exec -i -n "$NS" -c "$CONTAINER" "$POD" -- \
  python3.12 -c "
import tarfile, sys, shutil, os
dest = '/tmp/src'
if os.path.exists(dest):
    shutil.rmtree(dest)
os.makedirs(dest)
t = tarfile.open(fileobj=sys.stdin.buffer, mode='r|')
t.extractall(dest, filter='data')
t.close()
print('Extracted successfully to ' + dest)
"

echo ""
echo "Done. Connect to the container with:"
echo "  oc rsh -n ${NS} -c ${CONTAINER} ${POD}"
echo ""
echo "Then run the service with:"
echo "  PYTHONPATH=/tmp/src lightspeed-stack -c /vector-db-discovered-values/lightspeed-stack.yaml -v"
