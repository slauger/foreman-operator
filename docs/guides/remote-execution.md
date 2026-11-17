# Remote Execution

This guide enables Remote Execution and Ansible on an existing `Foreman` from [Getting Started](getting-started.md) and adds an `Execution` proxy that runs jobs over SSH. The resources follow the E2E test in `tests/e2e/mvp/resources.yaml`.

## Enable the plugins

`foreman_ansible` also enables `foreman_remote_execution` and `foreman-tasks`:

```yaml
apiVersion: platform.theforeman.org/v1alpha1
kind: Foreman
metadata:
  name: foreman
  namespace: foreman
spec:
  plugins:
    - foreman_ansible
```

A change of `spec.plugins` runs a migration like an upgrade: Foreman stops while the Job migrates and seeds the database, see [Upgrades](../concepts/upgrades.md). For many concurrent jobs, add workers with `dynflow.hostsQueueReplicas`.

Once enabled, `foreman-tasks`, `foreman_remote_execution`, and `foreman_ansible` cannot be disabled again, because the Dynflow orchestrator would fail on their scheduled tasks. See [Plugins](../concepts/plugins.md).

## Create the SSH key

The proxy connects to the hosts with the key in a Secret. Any key type works, despite the key names:

```bash
ssh-keygen -t ed25519 -N '' -C foreman-proxy -f rex
kubectl -n foreman create secret generic rex-ssh \
  --from-file=id_rsa=rex --from-file=id_rsa.pub=rex.pub
```

Add `rex.pub` to `authorized_keys` of the user Foreman connects as on every managed host, `root` by default.

## Create the proxy

An `Execution` proxy needs no Multus network and runs under Pod Security `restricted`:

```yaml
apiVersion: platform.theforeman.org/v1alpha1
kind: SmartProxy
metadata:
  name: rex
  namespace: foreman
spec:
  foremanRef: foreman
  profile: Execution
  url: https://rex.foreman.svc:8443
  tls:
    issuerRef:
      name: foreman-internal-ca
  features:
    remoteExecutionSSH:
      sshKeySecretRef: rex-ssh
    ansible: {}
```

The operator registers it in Foreman as `rex` with the features `Ansible`, `Dynflow`, `Logs`, and `Script`. The pod needs egress to the managed hosts on port 22.

## Run a job

By default, Foreman runs a job through any proxy with the `Script` feature (setting "Enable Global Proxy"). To control which proxy reaches which hosts, assign proxies to the subnets of the hosts. Foreman connects to the host name unless the host parameter `remote_execution_connect_by_ip` is `true`. Then run a job:

```bash
hammer job-invocation create --job-template "Run Command - Script Default" \
  --inputs command="uptime" --search-query "name = host.example.com"
```

## Limits

- Dynflow on the proxy keeps its state in memory. Jobs that run while the proxy pod is replaced fail.
- Pull mode over MQTT is not supported.
- When hosts call back to the proxy, its `url` must resolve from them. A `.svc` name works only for hosts inside the cluster.

See [Smart Proxy](../concepts/smart-proxy.md#execution-profile) for how the key is mounted.
