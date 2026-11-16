#!/usr/bin/env bash
# Upgrade test: installs a released operator in an empty namespace, creates data, upgrades the
# operator to IMAGE_TAG, and checks that Foreman passes the preflight, migrates, verifies, and
# keeps its data. Run setup.sh again before the MVP test afterwards.
set -euo pipefail

E2E_NAMESPACE="${E2E_NAMESPACE:-foreman-e2e}"
E2E_FQDN="${E2E_FQDN:-e2e-openvox.lnxlabs.de}"
FROM_VERSION="${FROM_VERSION:-0.1.0}"
IMAGE_TAG="${IMAGE_TAG:?IMAGE_TAG must name the image tag to upgrade to}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOMAIN="upgrade.example.com"

if ! kubectl get node e2e-openvox >/dev/null; then
    echo "refusing to run: not the e2e-openvox cluster" >&2
    exit 1
fi

foreman() {
    kubectl -n "${E2E_NAMESPACE}" get foreman foreman -o jsonpath="$1"
}

# wait_for <description> <timeout seconds> <command...> polls until the command succeeds.
wait_for() {
    local description="$1" timeout="$2"
    shift 2
    local deadline=$((SECONDS + timeout))
    until "$@" >/dev/null 2>&1; do
        if ((SECONDS > deadline)); then
            echo "timed out waiting for ${description}" >&2
            kubectl -n "${E2E_NAMESPACE}" get foreman,smartproxy,jobs,pods >&2 || true
            kubectl -n "${E2E_NAMESPACE}" get foreman foreman -o jsonpath='{.status}' >&2 || true
            exit 1
        fi
        sleep 10
    done
    echo "ok: ${description}"
}

api() {
    local password
    password="$(kubectl -n "${E2E_NAMESPACE}" get secret foreman-secrets -o jsonpath='{.data.admin-password}' | base64 -d)"
    curl -sfk -u "admin:${password}" -H 'Content-Type: application/json' "$@"
}

running() { [[ "$(foreman '{.status.phase}')" == "Running" ]]; }
proxy_ready() {
    [[ "$(kubectl -n "${E2E_NAMESPACE}" get smartproxy rex -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" == "True" ]]
}
domain_exists() {
    api "https://${E2E_FQDN}/api/v2/domains?search=name%3D${DOMAIN}" | jq -e '.subtotal == 1'
}

echo "== install ${FROM_VERSION} in an empty namespace"
kubectl delete namespace "${E2E_NAMESPACE}" --ignore-not-found --wait=true --timeout=900s
CHART_VERSION="${FROM_VERSION}" IMAGE_TAG="${FROM_VERSION}" "${SCRIPT_DIR}/setup.sh"
kubectl -n "${E2E_NAMESPACE}" apply -f "${SCRIPT_DIR}/upgrade/resources.yaml"
wait_for "Foreman ${FROM_VERSION} running" 1500 running
wait_for "proxy registered" 600 proxy_ready
api -X POST "https://${E2E_FQDN}/api/v2/domains" -d "{\"domain\":{\"name\":\"${DOMAIN}\"}}" >/dev/null
wait_for "test domain created" 60 domain_exists
old_hash="$(foreman '{.status.appliedMigrationHash}')"

echo "== upgrade the operator to ${IMAGE_TAG}"
IMAGE_TAG="${IMAGE_TAG}" "${SCRIPT_DIR}/setup.sh"
migrated() { [[ -n "$(foreman '{.status.appliedMigrationHash}')" && "$(foreman '{.status.appliedMigrationHash}')" != "${old_hash}" ]]; }
verified() {
    migrated && [[ "$(foreman '{.status.verifiedMigrationHash}')" == "$(foreman '{.status.appliedMigrationHash}')" ]] \
        && [[ "$(foreman '{.status.conditions[?(@.type=="Verified")].status}')" == "True" ]]
}
wait_for "migration to the new image" 1500 migrated
wait_for "smoke checks after the upgrade" 900 verified
running

version="$(foreman '{.status.foremanVersion}')"
[[ -n "${version}" ]] || { echo "status.foremanVersion is empty" >&2; exit 1; }
echo "ok: Foreman ${version}, $(foreman '{.status.conditions[?(@.type=="Verified")].message}')"
domain_exists >/dev/null || { echo "the test domain is gone after the upgrade" >&2; exit 1; }
echo "ok: data kept"
proxy_hash="$(kubectl -n "${E2E_NAMESPACE}" get smartproxy rex -o jsonpath='{.status.foremanMigrationHash}')"
[[ "${proxy_hash}" == "$(foreman '{.status.appliedMigrationHash}')" ]] || { echo "proxy did not refresh" >&2; exit 1; }
echo "ok: proxy refreshed its features"
echo "upgrade test passed"
