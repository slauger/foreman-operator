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

# shellcheck source=tests/e2e/lib.sh
source "${SCRIPT_DIR}/lib.sh"
require_e2e_cluster

echo "== install ${FROM_VERSION} in an empty namespace"
kubectl delete namespace "${E2E_NAMESPACE}" --ignore-not-found --wait=true --timeout=900s
CHART_VERSION="${FROM_VERSION}" IMAGE_TAG="${FROM_VERSION}" "${SCRIPT_DIR}/setup.sh"
kubectl -n "${E2E_NAMESPACE}" apply -f "${SCRIPT_DIR}/upgrade/resources.yaml"
wait_for "Foreman ${FROM_VERSION} running" 1500 running
wait_for "proxy registered" 600 proxy_ready
api -X POST "https://${E2E_FQDN}/api/v2/domains" -d "{\"domain\":{\"name\":\"${DOMAIN}\"}}" >/dev/null
wait_for "test domain created" 60 domain_exists "${DOMAIN}"
old_hash="$(foreman '{.status.appliedMigrationHash}')"

echo "== upgrade the operator to ${IMAGE_TAG}"
IMAGE_TAG="${IMAGE_TAG}" "${SCRIPT_DIR}/setup.sh"
migrated() { [[ -n "$(foreman '{.status.appliedMigrationHash}')" && "$(foreman '{.status.appliedMigrationHash}')" != "${old_hash}" ]]; }
wait_for "migration to the new image" 1500 migrated
wait_for "smoke checks after the upgrade" 900 verified
running

version="$(foreman '{.status.foremanVersion}')"
[[ -n "${version}" ]] || { echo "status.foremanVersion is empty" >&2; exit 1; }
echo "ok: Foreman ${version}, $(foreman '{.status.conditions[?(@.type=="Verified")].message}')"
domain_exists "${DOMAIN}" >/dev/null || { echo "the test domain is gone after the upgrade" >&2; exit 1; }
echo "ok: data kept"
proxy_hash="$(kubectl -n "${E2E_NAMESPACE}" get smartproxy rex -o jsonpath='{.status.foremanMigrationHash}')"
[[ "${proxy_hash}" == "$(foreman '{.status.appliedMigrationHash}')" ]] || { echo "proxy did not refresh" >&2; exit 1; }
echo "ok: proxy refreshed its features"
echo "upgrade test passed"
