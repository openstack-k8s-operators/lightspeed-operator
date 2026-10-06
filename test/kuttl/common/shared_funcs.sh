#!/bin/bash
#
# Shared assertion functions for kuttl test scripts.
#
# Usage: source ../../common/shared_funcs.sh
#   (from test/kuttl/tests/<test-name>/)

NAMESPACE="${NAMESPACE:-openstack-lightspeed}"
CR_NAME="${CR_NAME:-openstack-lightspeed}"

assert_mcp_openstack_enabled() {
  local expected="$1"
  local mcp_data
  mcp_data=$(oc get configmap mcp-config -n "$NAMESPACE" \
    -o jsonpath='{.data.config\.yaml}')
  local enabled
  enabled=$(echo "$mcp_data" | grep -A1 "openstack:" | grep "enabled:" | tr -d ' ')
  if [ "$enabled" != "enabled:${expected}" ]; then
    echo "ERROR: Expected MCP openstack enabled:${expected}, got ${enabled}"
    echo "$mcp_data"
    exit 1
  fi
}

assert_openstack_ready() {
  local expected="$1"
  local ready
  ready=$(oc get openstacklightspeed "$CR_NAME" -n "$NAMESPACE" \
    -o jsonpath='{.status.openStackReady}' 2>/dev/null || true)
  if [ "$expected" = "true" ]; then
    if [ "$ready" != "true" ]; then
      echo "ERROR: Expected openStackReady to be true, got '${ready}'"
      exit 1
    fi
  else
    if [ "$ready" = "true" ]; then
      echo "ERROR: Expected openStackReady to not be true, got '${ready}'"
      exit 1
    fi
  fi
}

assert_condition_status() {
  local condition_type="$1"
  local expected_status="$2"
  local actual
  actual=$(oc get openstacklightspeed "$CR_NAME" -n "$NAMESPACE" \
    -o jsonpath="{.status.conditions[?(@.type==\"${condition_type}\")].status}")
  if [ "$actual" != "$expected_status" ]; then
    echo "ERROR: Expected condition ${condition_type}=${expected_status}, got '${actual}'"
    oc get openstacklightspeed "$CR_NAME" -n "$NAMESPACE" -o yaml
    exit 1
  fi
}
