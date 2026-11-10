#!/usr/bin/env bash
# Checks the Foreman spike through the frontend with server and client TLS.
set -euo pipefail

NAMESPACE="foreman-spike"
HOST="e2e-openvox.lnxlabs.de"
PORT="18443"

export KUBECONFIG="${HOME}/.kube/e2e-openvox"

WORK="$(mktemp -d)"
trap 'kill "${PF_PID:-0}" 2>/dev/null || true; rm -rf "${WORK}"' EXIT

secret_field() {
    kubectl -n "${NAMESPACE}" get secret "$1" -o "jsonpath={.data.$2}" | base64 -d
}

secret_field frontend-server 'ca\.crt' >"${WORK}/ca.crt"
secret_field spike-client 'tls\.crt' >"${WORK}/client.crt"
secret_field spike-client 'tls\.key' >"${WORK}/client.key"
PASSWORD="$(secret_field foreman-secrets admin-password)"

kubectl -n "${NAMESPACE}" port-forward svc/frontend "${PORT}:8443" >/dev/null &
PF_PID=$!
sleep 3

CURL=(curl -sS --max-time 60 --cacert "${WORK}/ca.crt"
    --resolve "${HOST}:${PORT}:127.0.0.1"
    -H "Host: ${HOST}")

echo "--- ping"
"${CURL[@]}" "https://${HOST}:${PORT}/api/v2/ping"
echo
echo "--- login"
"${CURL[@]}" -o /dev/null -w '%{http_code}\n' -u "admin:${PASSWORD}" "https://${HOST}:${PORT}/api/v2/users"
echo "--- wrong password"
"${CURL[@]}" -o /dev/null -w '%{http_code}\n' -u "admin:wrong" "https://${HOST}:${PORT}/api/v2/users"
echo "--- with client certificate"
"${CURL[@]}" -o /dev/null -w '%{http_code}\n' --cert "${WORK}/client.crt" --key "${WORK}/client.key" \
    "https://${HOST}:${PORT}/api/v2/ping"
