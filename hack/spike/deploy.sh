#!/usr/bin/env bash
# Deploys the first Foreman spike into its own namespace on the e2e-openvox cluster.
set -euo pipefail

NAMESPACE="foreman-spike"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

export KUBECONFIG="${HOME}/.kube/e2e-openvox"

if [[ "$(kubectl config current-context)" != *e2e-openvox* ]] \
    && ! kubectl get node e2e-openvox >/dev/null 2>&1; then
    echo "refusing to run: not the e2e-openvox cluster" >&2
    exit 1
fi

kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
kubectl label namespace "${NAMESPACE}" --overwrite \
    pod-security.kubernetes.io/enforce=restricted

if ! kubectl -n "${NAMESPACE}" get secret foreman-secrets >/dev/null 2>&1; then
    kubectl -n "${NAMESPACE}" create secret generic foreman-secrets \
        --from-literal=encryption-key="$(openssl rand -hex 32)" \
        --from-literal=admin-password="$(openssl rand -hex 16)"
fi

helm upgrade --install foreman-db "${SCRIPT_DIR}/../../charts/foreman-db-cnpg" \
    --namespace "${NAMESPACE}" \
    --set storage.size=5Gi

kubectl -n "${NAMESPACE}" wait --for=condition=Ready cluster/foreman-db --timeout=300s

# A finished Job is immutable, so it is recreated on every run.
kubectl -n "${NAMESPACE}" delete job foreman-migrate --ignore-not-found
kubectl -n "${NAMESPACE}" apply -f "${SCRIPT_DIR}/manifests.yaml"
kubectl -n "${NAMESPACE}" wait --for=condition=Ready certificate --all --timeout=120s
kubectl -n "${NAMESPACE}" wait --for=condition=Complete job/foreman-migrate --timeout=1200s
kubectl -n "${NAMESPACE}" rollout restart deployment/foreman-web
kubectl -n "${NAMESPACE}" rollout status deployment/foreman-web --timeout=900s
kubectl -n "${NAMESPACE}" rollout status deployment/frontend --timeout=300s
