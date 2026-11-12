#!/usr/bin/env bash
# Runs a job template against the rex-target pod through the Execution proxy and prints the
# job status and the output of the command.
# Usage: run-job.sh <job template name> <command>
set -euo pipefail

template_name="$1"
command="$2"
base="https://e2e-openvox.lnxlabs.de/api"
password="$(kubectl -n foreman-e2e get secret foreman-secrets -o jsonpath='{.data.admin-password}' | base64 -d)"

api() {
    curl -sfk -u "admin:${password}" -H 'Content-Type: application/json' -H 'Accept: application/json' "$@"
}

# Foreman connects to the address of the host. The Service address stays the same when the
# pod is replaced.
ip="$(kubectl -n foreman-e2e get service rex-target -o jsonpath='{.spec.clusterIP}')"
host_id="$(api "${base}/hosts?search=name%3Drex-target" | jq -r '.results[0].id // empty')"
if [[ -z "${host_id}" ]]; then
    host_id="$(api -X POST "${base}/hosts" -d "$(jq -n --arg ip "${ip}" '{host: {
        name: "rex-target", managed: false, ip: $ip,
        host_parameters_attributes: [{name: "remote_execution_connect_by_ip", value: "true", parameter_type: "boolean"}]
    }}')" | jq -r .id)"
fi

template_id="$(api -G "${base}/job_templates" --data-urlencode "search=name = \"${template_name}\"" | jq -r '.results[0].id')"
job_id="$(api -X POST "${base}/job_invocations" -d "$(jq -n --argjson t "${template_id}" --arg c "${command}" '{job_invocation: {
    job_template_id: $t, targeting_type: "static_query", search_query: "name = rex-target",
    inputs: {command: $c}
}}')" | jq -r .id)"

status=""
for _ in $(seq 60); do
    status="$(api "${base}/job_invocations/${job_id}" | jq -r .status_label)"
    case "${status}" in
        succeeded | failed | cancelled) break ;;
    esac
    sleep 5
done
echo "${status}"
api "${base}/job_invocations/${job_id}/hosts/${host_id}" | jq -r '.output[]?.output' | tr -d '\r'
