#!/usr/bin/env bash
# Resolve the dev-latest digest (or take the override), validate it, and
# rewrite the repo-wide dev-latest@sha256:... pin.
#
# Required environment:
#   LCORE_IMAGE     - container image repository, e.g. quay.io/.../lightspeed-stack
# Optional environment:
#   OVERRIDE_DIGEST - pin this digest instead of resolving dev-latest via skopeo
#
# Step outputs (only set when something actually changed):
#   changed, digest, short

set -euo pipefail

if [[ -n "${OVERRIDE_DIGEST:-}" ]]; then
  new_digest="$OVERRIDE_DIGEST"
else
  new_digest="$(skopeo inspect --format '{{.Digest}}' "docker://${LCORE_IMAGE}:dev-latest")"
fi
[[ "$new_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || { echo "::error::Invalid digest: ${new_digest}"; exit 1; }

# Replace any existing dev-latest@sha256:... pin across tracked files
git grep -lzE 'dev-latest@sha256:[0-9a-f]{64}' \
  | xargs -0 sed -i -E "s|dev-latest@sha256:[0-9a-f]{64}|dev-latest@${new_digest}|g"

# Check git diff to see if anything actually changed
if ! git diff --quiet; then
  {
    echo "changed=true"
    echo "digest=${new_digest}"
    echo "short=${new_digest:7:12}"
  } >> "${GITHUB_OUTPUT:?}"
fi
