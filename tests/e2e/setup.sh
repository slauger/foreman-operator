#!/usr/bin/env bash
# Prepares the e2e-openvox cluster: prerequisites in the test namespace and the operator.
set -euo pipefail

E2E_NAMESPACE="${E2E_NAMESPACE:-foreman-e2e}"
E2E_FQDN="${E2E_FQDN:-e2e-openvox.lnxlabs.de}"
E2E_EXTERNAL_IP="${E2E_EXTERNAL_IP:-188.245.216.177}"
IMAGE_TAG="${IMAGE_TAG:?IMAGE_TAG must name the image tag to test}"
# OPERATOR=local skips the in-cluster operator, e.g. for "make run" during development.
OPERATOR="${OPERATOR:-helm}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
export E2E_NAMESPACE E2E_FQDN E2E_EXTERNAL_IP

# The connection to the API server can drop, and every step below is safe to repeat.
retry() {
    local attempt
    for attempt in 1 2 3 4 5 6 7 8; do
        if "$@"; then
            return 0
        fi
        echo "retry ${attempt}: $*" >&2
        sleep 10
    done
    return 1
}

if ! retry kubectl get node e2e-openvox >/dev/null; then
    echo "refusing to run: not the e2e-openvox cluster" >&2
    exit 1
fi

kubectl create namespace "${E2E_NAMESPACE}" --dry-run=client -o yaml | retry kubectl apply -f -
# dnsmasq in the Smart Proxy pod needs NET_ADMIN, which only the privileged level allows.
retry kubectl label namespace "${E2E_NAMESPACE}" --overwrite \
    pod-security.kubernetes.io/enforce=privileged \
    pod-security.kubernetes.io/warn=restricted

if ! retry kubectl -n "${E2E_NAMESPACE}" get secret tsig >/dev/null 2>&1; then
    secret="$(openssl rand -base64 32)"
    printf 'key "foreman" {\n  algorithm hmac-sha256;\n  secret "%s";\n};\n' "${secret}" > "${TMPDIR:-/tmp}/tsig-key"
    retry kubectl -n "${E2E_NAMESPACE}" create secret generic tsig --from-file=key="${TMPDIR:-/tmp}/tsig-key"
    rm -f "${TMPDIR:-/tmp}/tsig-key"
fi

retry helm upgrade --install foreman-db "${ROOT_DIR}/charts/foreman-db-cnpg" \
    --namespace "${E2E_NAMESPACE}" --set storage.size=5Gi
retry kubectl -n "${E2E_NAMESPACE}" apply -f "${SCRIPT_DIR}/setup/prerequisites.yaml"
envsubst < "${SCRIPT_DIR}/setup/gateway.yaml" | retry kubectl -n "${E2E_NAMESPACE}" apply -f -

# Helm installs CRDs only once, so they are applied explicitly.
retry kubectl apply --server-side --force-conflicts -f "${ROOT_DIR}/charts/foreman-operator/crds/"
if [[ "${OPERATOR}" == "helm" ]]; then
    retry helm upgrade --install foreman-operator "${ROOT_DIR}/charts/foreman-operator" \
        --namespace foreman-operator --create-namespace \
        --set image.tag="${IMAGE_TAG}" \
        --set image.pullPolicy=Always \
        --set operator.defaultImageTag="${IMAGE_TAG}"
    retry kubectl -n foreman-operator rollout restart deployment/foreman-operator
    retry kubectl -n foreman-operator rollout status deployment/foreman-operator --timeout=300s
    # The new pod reconciles only after the lease of the old one expires. Until then, the
    # status of the resources still reflects the previous operator version.
    pod="$(kubectl -n foreman-operator get pods -l app.kubernetes.io/name=foreman-operator -o json \
        | jq -r '[.items[] | select(.metadata.deletionTimestamp == null)][0].metadata.name')"
    for _ in $(seq 60); do
        holder="$(kubectl -n foreman-operator get lease foreman-operator.platform.theforeman.org \
            -o jsonpath='{.spec.holderIdentity}' 2>/dev/null || true)"
        [[ "${holder}" == "${pod}_"* ]] && break
        sleep 5
    done
    # Give the new leader time to reconcile and publish the current status.
    sleep 15
fi

retry kubectl -n "${E2E_NAMESPACE}" wait --for=condition=Ready cluster/foreman-db --timeout=600s
retry kubectl -n "${E2E_NAMESPACE}" wait --for=condition=Ready certificate/foreman-internal-ca --timeout=120s
