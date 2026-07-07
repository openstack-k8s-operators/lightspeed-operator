#!/bin/bash
# Remove images from the OpenShift internal image registry.
#
# Deletes the ImageStream resources corresponding to the given images.
#
# Usage: ocp-registry-clean.sh <image1> [image2 ...]
#
# Each <imageN> must use the in-cluster registry address, e.g.:
#   image-registry.openshift-image-registry.svc:5000/my-ns/my-image:tag
set -euo pipefail

if [ $# -eq 0 ]; then
  echo "Usage: $0 <image1> [image2 ...]"
  echo "Error: at least one image must be specified"
  exit 1
fi

INTERNAL_PREFIX="image-registry.openshift-image-registry.svc:5000"

for IMAGE in "$@"; do
  IMAGE_PATH="${IMAGE#"${INTERNAL_PREFIX}"/}"
  if [ "${IMAGE_PATH}" = "${IMAGE}" ]; then
    echo "Warning: image '${IMAGE}' does not start with '${INTERNAL_PREFIX}', skipping."
    continue
  fi

  NAMESPACE="${IMAGE_PATH%%/*}"
  NAME_TAG="${IMAGE_PATH#*/}"
  NAME="${NAME_TAG%%:*}"

  echo "==> Deleting ImageStream '${NAME}' in namespace '${NAMESPACE}'..."
  oc delete imagestream "${NAME}" -n "${NAMESPACE}" --ignore-not-found=true
done

echo "==> Registry cleanup complete."
