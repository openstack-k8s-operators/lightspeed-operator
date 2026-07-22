#!/bin/bash
# Simulates what the keystone-operator would do when processing a
# KeystoneApplicationCredential CR: create an AC secret and update the
# CR status with the secret name.
set -euo pipefail

NAMESPACE="openstack"
AC_CR_NAME="lightspeed"
AC_SECRET_NAME="ac-lightspeed-test-secret"
AC_ID="mock-ac-id-12345"
AC_SECRET="mock-ac-secret-abcde"

echo "Waiting for KeystoneApplicationCredential CR to exist..."
for i in $(seq 1 60); do
  if oc get keystoneapplicationcredentials.keystone.openstack.org "$AC_CR_NAME" \
       -n "$NAMESPACE" 2>/dev/null; then
    echo "AC CR found"
    break
  fi
  if [ "$i" -eq 60 ]; then
    echo "ERROR: AC CR not found after 120s"
    exit 1
  fi
  sleep 2
done

echo "Creating AC secret..."
oc apply -f - <<EOF
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: ${AC_SECRET_NAME}
  namespace: ${NAMESPACE}
stringData:
  AC_ID: "${AC_ID}"
  AC_SECRET: "${AC_SECRET}"
EOF

echo "Patching AC CR status with secretName..."
oc patch keystoneapplicationcredentials.keystone.openstack.org "$AC_CR_NAME" \
  -n "$NAMESPACE" \
  --type merge \
  --subresource status \
  -p "{\"status\":{\"secretName\":\"${AC_SECRET_NAME}\"}}"

echo "Keystone operator simulation complete"
