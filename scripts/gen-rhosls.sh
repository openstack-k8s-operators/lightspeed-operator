#!/bin/bash
# Deploy openstack-lightspeed using the catalog
# Optionally pass the catalog name and channel to use
set -ex

if [ -z "${OUTPUT_DIR}" ]; then
  echo "Please set OPERATOR_DIR"
  exit 1
fi

CATALOG=${1:-openstack-lightspeed-catalog}
CHANNEL=${2:-alpha}

DEST_DIR="${OUTPUT_DIR}/rhosls"
mkdir -p "${DEST_DIR}"

cat > "${DEST_DIR}/namespace.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: openstack-lightspeed
EOF

cat > "${DEST_DIR}/operator_group.yaml" <<EOF
apiVersion: operators.coreos.com/v1
kind: OperatorGroup
metadata:
  name: lightspeed-operator-group
  namespace: openstack-lightspeed
spec:
  targetNamespaces:
  - openstack-lightspeed
EOF

for _i in $(seq 1 20); do
  CSV_VERSION=$(oc get --ignore-not-found=true packagemanifest \
    --field-selector "metadata.name=openstack-lightspeed-operator" \
    -l "catalog=${CATALOG}" \
    -o go-template="{{range .items}}{{range .status.channels}}{{if eq .name \"${CHANNEL}\"}}{{.currentCSV}}{{\"\n\"}}{{end}}{{end}}{{end}}")
  if [ -n "${CSV_VERSION}" ]; then
    break
  fi
  sleep 2
done
if [ -z "${CSV_VERSION}" ]; then
  exit 1
fi

cat > "${DEST_DIR}/subscription.yaml" <<EOF
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  labels:
    operators.coreos.com/lightspeed-operator.openstack-lightspeed: ""
  name: openstack-lightspeed-operator
  namespace: openstack-lightspeed
spec:
  channel: ${CHANNEL}
  installPlanApproval: Automatic
  name: openstack-lightspeed-operator
  source: ${CATALOG}
  sourceNamespace: openshift-marketplace
  startingCSV: ${CSV_VERSION}
EOF
