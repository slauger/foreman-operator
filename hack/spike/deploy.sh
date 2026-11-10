#!/usr/bin/env bash
# Deploys the first Foreman spike into its own namespace on the e2e-openvox cluster.
set -euo pipefail

NAMESPACE="foreman-spike"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export KUBECONFIG="${HOME}/.kube/e2e-openvox"

# The connection to the API server can drop, and every step below is safe to repeat.
retry() {
    local attempt
    for attempt in 1 2 3 4 5 6 7 8; do
        if "$@"; then
            return 0
        fi
        echo "retry ${attempt}: $*" >&2
        sleep 15
    done
    return 1
}

k() {
    retry kubectl -n "${NAMESPACE}" "$@"
}

if ! retry kubectl get node e2e-openvox >/dev/null; then
    echo "refusing to run: not the e2e-openvox cluster" >&2
    exit 1
fi

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | retry kubectl apply --validate=false -f -
retry kubectl label namespace "${NAMESPACE}" --overwrite \
    pod-security.kubernetes.io/enforce=restricted

if ! k get secret foreman-secrets >/dev/null 2>&1; then
    k create secret generic foreman-secrets \
        --from-literal=encryption-key="$(openssl rand -hex 32)" \
        --from-literal=secret-key-base="$(openssl rand -hex 64)" \
        --from-literal=admin-password="$(openssl rand -hex 16)"
fi

retry helm upgrade --install foreman-db "${SCRIPT_DIR}/../../charts/foreman-db-cnpg" \
    --namespace "${NAMESPACE}" \
    --set storage.size=5Gi

k wait --for=condition=Ready cluster/foreman-db --timeout=300s

# A finished Job is immutable, so it is recreated on every run.
k delete job foreman-migrate --ignore-not-found
k apply --validate=false -f "${SCRIPT_DIR}/manifests.yaml"
k apply --validate=false -f "${SCRIPT_DIR}/gateway.yaml"
k wait --for=condition=Ready certificate --all --timeout=120s

for _ in $(seq 1 120); do
    if kubectl -n "${NAMESPACE}" wait --for=condition=Complete job/foreman-migrate --timeout=1s >/dev/null 2>&1; then
        break
    fi
    if kubectl -n "${NAMESPACE}" wait --for=condition=Failed job/foreman-migrate --timeout=1s >/dev/null 2>&1; then
        k logs job/foreman-migrate --tail=40 >&2
        echo "migration failed" >&2
        exit 1
    fi
    sleep 10
done
k wait --for=condition=Complete job/foreman-migrate --timeout=1s

k rollout restart deployment/foreman-web deployment/dynflow-orchestrator deployment/dynflow-worker deployment/frontend
k rollout status deployment/dynflow-orchestrator --timeout=600s
k rollout status deployment/dynflow-worker --timeout=600s
k rollout status deployment/foreman-web --timeout=900s
k rollout status deployment/frontend --timeout=300s
