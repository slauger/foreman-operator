#!/usr/bin/env bash
# Restore drill: installs IMAGE_TAG in an empty namespace and creates data, then backs up the
# database with pg_dump and the Foreman secrets, deletes the Foreman, the SmartProxy, the
# database cluster, and the secrets, and restores them in the documented order. It checks
# that Foreman runs with its data, decrypts encrypted columns, and adopts the proxy
# registration. Run setup.sh again before the MVP test afterwards.
set -euo pipefail

E2E_NAMESPACE="${E2E_NAMESPACE:-foreman-e2e}"
E2E_FQDN="${E2E_FQDN:-e2e-openvox.lnxlabs.de}"
IMAGE_TAG="${IMAGE_TAG:?IMAGE_TAG must name the image tag to test}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DOMAIN="restore.example.com"
LDAP_SECRET="restore-drill-secret"

# shellcheck source=tests/e2e/lib.sh
source "${SCRIPT_DIR}/lib.sh"
require_e2e_cluster

backup_dir="$(mktemp -d)"
trap 'rm -rf "${backup_dir}"' EXIT
ns=(kubectl -n "${E2E_NAMESPACE}")

ldap_password() {
    "${ns[@]}" exec deploy/foreman-web -c foreman -- \
        bin/rails runner 'print AuthSourceLdap.find_by(name: "restore-drill")&.account_password' 2>/dev/null | tail -1
}

echo "== install ${IMAGE_TAG} in an empty namespace and create data"
kubectl delete namespace "${E2E_NAMESPACE}" --ignore-not-found --wait=true --timeout=900s
IMAGE_TAG="${IMAGE_TAG}" "${SCRIPT_DIR}/setup.sh"
"${ns[@]}" apply -f "${SCRIPT_DIR}/upgrade/resources.yaml"
wait_for "Foreman running" 1500 running
wait_for "proxy registered" 600 proxy_ready
proxy_id="$("${ns[@]}" get smartproxy rex -o jsonpath='{.status.foremanID}')"
api -X POST "https://${E2E_FQDN}/api/v2/domains" -d "{\"domain\":{\"name\":\"${DOMAIN}\"}}" >/dev/null
# The account password of an LDAP source is an encrypted column, readable only with the
# original ENCRYPTION_KEY. Creating the source does not contact the server.
api -X POST "https://${E2E_FQDN}/api/v2/auth_source_ldaps" -d '{"auth_source_ldap":{"name":"restore-drill",
    "host":"ldap.example.com","port":389,"server_type":"posix","account":"cn=reader,dc=example,dc=com",
    "account_password":"'"${LDAP_SECRET}"'","base_dn":"dc=example,dc=com","attr_login":"uid","onthefly_register":false}}' >/dev/null
[[ "$(ldap_password)" == "${LDAP_SECRET}" ]] || { echo "the LDAP password cannot be read before the backup" >&2; exit 1; }
echo "ok: data created"

echo "== back up the database and the Foreman secrets"
"${ns[@]}" exec foreman-db-1 -c postgres -- pg_dump -Fc -d foreman > "${backup_dir}/foreman.dump"
"${ns[@]}" get secret foreman-secrets -o json \
    | jq 'del(.metadata.uid, .metadata.resourceVersion, .metadata.creationTimestamp, .metadata.managedFields, .metadata.ownerReferences)' \
    > "${backup_dir}/foreman-secrets.json"
echo "ok: dump of $(wc -c < "${backup_dir}/foreman.dump") bytes"

echo "== lose the Foreman, the proxy, the database, and the secrets"
# The resources are removed without the operator, as after losing the namespace contents,
# so the proxy stays registered in the backup.
kubectl -n foreman-operator scale deployment/foreman-operator --replicas=0
kubectl -n foreman-operator rollout status deployment/foreman-operator --timeout=120s
"${ns[@]}" patch smartproxy rex --type=merge -p '{"metadata":{"finalizers":null}}'
"${ns[@]}" delete smartproxy rex --wait=true
"${ns[@]}" delete foreman foreman --wait=true
helm uninstall foreman-db --namespace "${E2E_NAMESPACE}" --wait
"${ns[@]}" delete secret foreman-secrets
wait_for "database volumes deleted" 300 bash -c "! kubectl -n '${E2E_NAMESPACE}' get pvc -l cnpg.io/cluster=foreman-db -o name | grep -q ."

echo "== restore: secrets, database, then the resources"
"${ns[@]}" apply -f "${backup_dir}/foreman-secrets.json"
IMAGE_TAG="${IMAGE_TAG}" "${SCRIPT_DIR}/setup.sh"
"${ns[@]}" exec -i foreman-db-1 -c postgres -- pg_restore -d foreman --no-owner --role=app --exit-on-error \
    < "${backup_dir}/foreman.dump"
echo "ok: database restored"
"${ns[@]}" apply -f "${SCRIPT_DIR}/upgrade/resources.yaml"
wait_for "Foreman running on the restored database" 1500 running
wait_for "smoke checks after the restore" 900 verified

domain_exists "${DOMAIN}" >/dev/null || { echo "the test domain is gone after the restore" >&2; exit 1; }
echo "ok: data restored"
[[ "$(ldap_password)" == "${LDAP_SECRET}" ]] || { echo "the encrypted LDAP password cannot be read after the restore" >&2; exit 1; }
echo "ok: encrypted columns readable with the restored secrets"
[[ "$("${ns[@]}" get smartproxy rex -o jsonpath='{.status.foremanID}')" == "${proxy_id}" ]] \
    || { echo "the proxy registered again instead of adopting its registration" >&2; exit 1; }
echo "ok: proxy adopted its registration ${proxy_id}"
echo "restore drill passed"
