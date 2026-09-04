#!/usr/bin/env bash
# KUTTL failure collector for OpenStack Lightspeed. It runs the following commands
# against the openstack-lightspeed namespace (configurable via KUTTL_NAMESPACE)
# and writes the results either to stdout or, when KUTTL_COLLECTOR_OUTPUT is set
# to `file`, to KUTTL_COLLECTOR_FILE_PATH (default:
# ../../../../kuttl-collectors-output.log):
#
#   * oc describe {deployment,replicaset,pod,pvc} -n openstack-lightspeed
#   * oc get events -n openstack-lightspeed
#   * oc logs {pod names in openstack-lightspeed} -n openstack-lightspeed
#
# This script is executed when collectors are configured for a TestAssert as follows.
# Note that collectors are triggered only when the TestAssert fails:
#
#   ---
#   apiVersion: kuttl.dev/v1beta1
#   kind: TestAssert
#   collectors:
#     - type: command
#       command: ../../common/collectors/collect-workload-diagnostics.sh
#
# You can also run this script standalone, provided that `oc` is installed and
# configured to access an OpenShift cluster:
#
#    ./test/kuttl/common/collectors/collect-workload-diagnostics.sh

set -u

if [[ "${KUTTL_COLLECTOR_OUTPUT:-}" == "file" ]]; then
  exec > "${KUTTL_COLLECTOR_FILE_PATH:-../../../../kuttl-collectors-output-$(date +%Y%m%d-%H%M%S).log}" 2>&1
fi

namespace="${KUTTL_NAMESPACE:-openstack-lightspeed}"
resources=(deployment replicaset pod pvc)

run() {
  echo
  echo "===== $* ====="
  "$@" || true
}

resource_names() {
  # A failed list request should not stop subsequent diagnostic collection.
  oc get "$1" -n "${namespace}" -o name 2>/dev/null || true
}

describe_resources() {
  local resource="$1"
  local name

  while IFS= read -r name; do
    [[ -z "${name}" ]] && continue
    run oc describe -n "${namespace}" "${name}"
  done < <(resource_names "${resource}")
}

collect_container_logs() {
  local pod="$1"
  local container

  while IFS= read -r container; do
    [[ -z "${container}" ]] && continue
    run oc logs -n "${namespace}" "${pod}" -c "${container}" --tail=-1
    run oc logs -n "${namespace}" "${pod}" -c "${container}" --previous --tail=-1
  done < <(
    oc get "${pod}" -n "${namespace}" \
      -o jsonpath='{range .spec.initContainers[*]}{.name}{"\n"}{end}{range .spec.containers[*]}{.name}{"\n"}{end}{range .spec.ephemeralContainers[*]}{.name}{"\n"}{end}' \
      2>/dev/null || true
  )
}

echo "$(date '+%Y-%m-%dT%H:%M:%S%z') Collecting OpenStack Lightspeed diagnostics from namespace ${namespace}"

run oc get events -n "${namespace}" --sort-by=.lastTimestamp
run oc get deployment,replicaset,pod,pvc -n "${namespace}" -o wide

for resource in "${resources[@]}"; do
  describe_resources "${resource}"
done

while IFS= read -r pod; do
  [[ -z "${pod}" ]] && continue
  collect_container_logs "${pod}"
done < <(resource_names pod)
