#!/bin/bash
set -euo pipefail

# Only remove the configuration created by run_with_local_webhook.sh.
"${OC:-oc}" delete validatingwebhookconfiguration \
    openstack-lightspeed-local-webhook --ignore-not-found
