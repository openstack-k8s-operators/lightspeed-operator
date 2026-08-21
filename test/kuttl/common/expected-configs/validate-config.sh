#!/bin/bash

# validate-config.sh
#
# Common script for validating OpenStack Lightspeed configs. It compares the
# configs against expected config files for a given test.
#
# Usage: validate-config.sh <config-type> <expected-config-path>
#   config-type: lightspeed-stack or ogx_config
#   expected-config-path: path to expected config file (relative from test dir)

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "Usage: $0 <config-type> <expected-config-path>"
  echo "  config-type: lightspeed-stack or ogx_config"
  exit 1
fi

CONFIG_TYPE="$1"
EXPECTED_CONFIG="$2"

case "$CONFIG_TYPE" in
  lightspeed-stack)
    CONTAINER="lightspeed-service-api"
    POD_PATH="/vector-db-discovered-values/lightspeed-stack.yaml"
    ;;
  ogx_config)
    CONTAINER="ogx"
    POD_PATH="/vector-db-discovered-values/ogx_config.yaml"
    ;;
  *)
    echo "ERROR: Invalid config type '$CONFIG_TYPE'"
    echo "Valid types: lightspeed-stack, ogx_config"
    exit 1
    ;;
esac

# Create dedicated temporary directory for KUTTL tests
KUTTL_TEMP_DIR="$PWD/.kuttl-tests-tmp"
mkdir -p "$KUTTL_TEMP_DIR"

cleanup() {
  if [ -d "$KUTTL_TEMP_DIR" ]; then
    echo "Cleaning up temporary files..."
    rm -f "$KUTTL_TEMP_DIR"/actual-"${CONFIG_TYPE}"-*.yaml 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

# Generate temp file names with unique timestamp
TIMESTAMP="$(date +%s)-$$"
TEMP_ACTUAL="$KUTTL_TEMP_DIR/actual-${CONFIG_TYPE}-${TIMESTAMP}.yaml"
TEMP_ACTUAL_NORM="$KUTTL_TEMP_DIR/actual-${CONFIG_TYPE}-${TIMESTAMP}-norm.yaml"

echo "Waiting for lightspeed-stack-deployment pod to be ready..."
oc wait --for=condition=Ready pod \
  -l app.kubernetes.io/name=openstack-lightspeed-app-server \
  -n openstack-lightspeed \
  --timeout=120s

echo "Getting pod name..."
POD_NAMES="$(oc get pods \
  -l app.kubernetes.io/name=openstack-lightspeed-app-server \
  -n openstack-lightspeed \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')"

POD_COUNT="$(printf '%s\n' "$POD_NAMES" | sed '/^$/d' | wc -l | tr -d ' ')"

if [ "$POD_COUNT" -ne 1 ]; then
  echo "ERROR: Expected exactly one pod, found $POD_COUNT"
  exit 1
fi

POD_NAME="$(printf '%s\n' "$POD_NAMES" | sed -n '1p')"

echo "Extracting $CONFIG_TYPE from pod $POD_NAME (container: $CONTAINER)..."
oc exec -i -n openstack-lightspeed "$POD_NAME" \
  -c "$CONTAINER" -- cat "$POD_PATH" > "$TEMP_ACTUAL"

# Normalize values that vary between vector DB container images—such as dynamically
# generated UUIDs—to ensure tests remain stable even when new images are used.
echo "Normalizing dynamic UUIDs in actual config for comparison..."
sed -E '
  s/kv_rag_[A-Za-z0-9]{10}_/kv_rag_UUID_/g;
  s/\/[A-Za-z0-9]{10}\//\/UUID\//g;
  s/vs_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/vs_UUID/g
' "$TEMP_ACTUAL" > "$TEMP_ACTUAL_NORM"

echo "Comparing with expected config..."
if ! diff -u "$EXPECTED_CONFIG" "$TEMP_ACTUAL_NORM"; then
  echo ""
  echo "ERROR: ${CONFIG_TYPE} config mismatch!"
  echo "Expected config: $EXPECTED_CONFIG"
  echo "Actual config from pod: $POD_NAME (container: $CONTAINER)"
  echo "Path: $POD_PATH"
  echo "Note: Dynamic UUIDs in actual config are normalized for comparison"
  exit 1
fi

echo "✓ ${CONFIG_TYPE} config matches expected"
