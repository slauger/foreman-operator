# Smart Proxy

!!! note "Status"
    Target design. The `SmartProxy` CRD exists. No controller or image exists yet.

Each `SmartProxy` is one proxy registered in Foreman and always has one replica. Its state belongs to one pod: the Dynflow core keeps state in memory by default, and provisioning data lives on a volume.

## Identity and registration

| Setting | Purpose |
|---|---|
| `spec.url` | The URL registered in Foreman. Foreman validates the proxy certificate against this hostname. |
| `trusted_hosts` | Holds the CN of Foreman's client certificate, which equals `Foreman.spec.fqdn`. |
| `permitted_hosts` | Lists the names used to reach the proxy. |

The operator registers the proxy through the Foreman API with the OAuth credentials from `spec.oauthSecretRef`, and removes it through a finalizer.

## Profiles

| Profile | Features | Network | Privileges |
|---|---|---|---|
| `Provisioning` | `tftp`, `dhcp`, `dns`, `bmc`, `templates`, `httpBoot` | Multus | Dedicated, see [Provisioning](provisioning.md) |
| `Execution` | `remoteExecutionSSH`, `ansible` | Pod network with egress to hosts | None beyond the default |

A third profile for Katello content mirrors will be designed with [Katello](katello.md).

`Execution` needs persistent SSH keys in a Secret. Its URL must resolve from managed hosts when callbacks are used.

## One pod per proxy

Every `SmartProxy` is its own pod. Foreman assigns the proxy per feature on each subnet, so a deployment can split features across several small proxies instead of one large one:

| Proxy | Features | Network | Privileges |
|---|---|---|---|
| One per L2 network | `dhcp`, `tftp`, `templates` | Multus | Dedicated |
| One per organization or zone | `dns` | Pod network | Default |
| One per BMC network, where needed | `bmc` | Pod network or Multus | Default |
| One or more | `remoteExecutionSSH`, `ansible` | Pod network | Default |

A feature and the service it controls share a pod, because they share files. DHCP and TFTP stay together since dnsmasq serves both.

## Off-cluster proxies

A proxy outside the cluster cannot use cert-manager directly. The operator will export a certificate bundle for it, similar to the `auth-bundle` command of `foremanctl`.

Environments in which Foreman provisions the nodes of its own cluster must keep a proxy outside that cluster. A proxy inside the cluster cannot rebuild it after a failure.
