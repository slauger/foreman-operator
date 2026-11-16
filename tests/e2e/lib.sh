# Helpers shared by the lifecycle drills upgrade.sh and restore.sh. Source it after setting
# E2E_NAMESPACE and E2E_FQDN.

require_e2e_cluster() {
    if ! kubectl get node e2e-openvox >/dev/null; then
        echo "refusing to run: not the e2e-openvox cluster" >&2
        exit 1
    fi
}

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

verified() {
    [[ -n "$(foreman '{.status.appliedMigrationHash}')" ]] \
        && [[ "$(foreman '{.status.verifiedMigrationHash}')" == "$(foreman '{.status.appliedMigrationHash}')" ]] \
        && [[ "$(foreman '{.status.conditions[?(@.type=="Verified")].status}')" == "True" ]]
}

proxy_ready() {
    [[ "$(kubectl -n "${E2E_NAMESPACE}" get smartproxy rex -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')" == "True" ]]
}

# domain_exists <name> checks the domain through the API.
domain_exists() {
    api "https://${E2E_FQDN}/api/v2/domains?search=name%3D$1" | jq -e '.subtotal == 1'
}
