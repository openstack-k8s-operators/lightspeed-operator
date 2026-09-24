#!/bin/bash
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")/.."
# shellcheck source=scripts/env.sh
source scripts/env.sh

OC=${OC:-oc}
WEBHOOK_PORT=${WEBHOOK_PORT:-9443}
HEALTH_PORT=${HEALTH_PORT:-8081}
WEBHOOK_CERT_DIR=${WEBHOOK_CERT_DIR:-"$PWD/bin/local-webhook"}
webhook_name=openstack-lightspeed-local-webhook

for command in "$OC" openssl jq curl ip; do
    command -v "$command" >/dev/null || { echo "Required command not found: $command" >&2; exit 1; }
done

# Use the host-side CRC bridge address reachable from the VM.
crc_host_ip=$(ip -o -4 addr show dev crc 2>/dev/null | awk '{split($4, address, "/"); print address[1]; exit}' || true)
if [[ -z "$crc_host_ip" ]]; then
    echo "No IPv4 address found on the crc interface. This helper requires a local Linux CRC cluster." >&2
    exit 1
fi
if [[ -z "$WATCH_NAMESPACE" || "$WATCH_NAMESPACE" == *,* ]]; then
    echo "Local webhook development requires one WATCH_NAMESPACE." >&2
    exit 1
fi

# Avoid running alongside another Lightspeed webhook, including one managed by OLM.
existing=$("$OC" get validatingwebhookconfigurations -o json | jq -r '
    .items[] | select(.metadata.name == "openstack-lightspeed-local-webhook" or
        any(.webhooks[]; .name == "vopenstacklightspeed-v1beta1.kb.io")) | .metadata.name')
if [[ -n "$existing" ]]; then
    echo "A Lightspeed webhook is already installed: $existing" >&2
    echo "Use make webhook-cleanup for a stale local webhook; uninstall the deployed operator before running locally." >&2
    exit 1
fi

umask 077
mkdir -p "$WEBHOOK_CERT_DIR"
openssl req -newkey rsa:2048 -nodes -x509 -days 30 \
    -subj "/CN=lightspeed-local-webhook" -addext "subjectAltName=IP:$crc_host_ip" \
    -keyout "$WEBHOOK_CERT_DIR/tls.key" -out "$WEBHOOK_CERT_DIR/tls.crt"

# The API server needs only the public certificate, never the private key.
ca_bundle=$(openssl base64 -A -in "$WEBHOOK_CERT_DIR/tls.crt")
"$OC" create --dry-run=client --validate=false -f config/webhook/manifests.yaml -o json | \
    jq --arg name "$webhook_name" --arg url "https://$crc_host_ip:$WEBHOOK_PORT" \
       --arg ca "$ca_bundle" --arg namespace "$WATCH_NAMESPACE" '
        .metadata.name = $name |
        .webhooks |= map(
            .clientConfig = {url: ($url + .clientConfig.service.path), caBundle: $ca} |
            .namespaceSelector = {matchLabels: {"kubernetes.io/metadata.name": $namespace}}
        )' > "$WEBHOOK_CERT_DIR/webhook.json"

manager_pid=""
registered=false
# Invoked indirectly by the EXIT trap.
# shellcheck disable=SC2317,SC2329
cleanup() {
    local status=$?
    trap - EXIT INT TERM
    if [[ "$registered" == true ]]; then
        bash hack/clean_local_webhook.sh || status=1
    fi
    if [[ -n "$manager_pid" ]]; then
        kill "$manager_pid" 2>/dev/null || true
        wait "$manager_pid" 2>/dev/null || true
    fi
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

export ENABLE_WEBHOOKS=true OC
./bin/manager "$@" --webhook-port="$WEBHOOK_PORT" --webhook-cert-path="$WEBHOOK_CERT_DIR" \
    --health-probe-bind-address="127.0.0.1:$HEALTH_PORT" &
manager_pid=$!

# Register only after the local server is ready to answer admission requests.
for ((attempt=0; attempt<60; attempt++)); do
    if ! kill -0 "$manager_pid" 2>/dev/null; then
        wait "$manager_pid"
        exit 1
    fi
    if curl --noproxy '*' --fail --silent --max-time 1 "http://127.0.0.1:$HEALTH_PORT/readyz/webhook" >/dev/null; then
        registered=true
        "$OC" apply -f "$WEBHOOK_CERT_DIR/webhook.json"
        echo "Local webhook registered at https://$crc_host_ip:$WEBHOOK_PORT for namespace $WATCH_NAMESPACE."
        echo "The API server must be able to reach this address and port. Press Ctrl+C to stop and clean up."
        wait "$manager_pid"
        exit 0
    fi
    sleep 1
done
echo "Timed out waiting for the local webhook server to become ready." >&2
exit 1
