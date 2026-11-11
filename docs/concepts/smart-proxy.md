# Smart Proxy

!!! note "Status"
    The `SmartProxy` controller and image are implemented for the `Provisioning` profile. Registration through OAuth is covered by the E2E test, but has not been run on a cluster yet; the OAuth signature itself was verified against a real Foreman. The `Execution` profile and off-cluster proxies are target design.

Each `SmartProxy` is one proxy registered in Foreman and always has one replica. Its state belongs to one pod: the Dynflow core keeps state in memory by default, and the TFTP root lives on a volume.

## Identity

| Setting | Purpose |
|---|---|
| `spec.url` | The URL registered in Foreman. Foreman validates the proxy certificate against this hostname. |
| `trusted_hosts` | Holds the CN of Foreman's client certificate, which equals `Foreman.spec.fqdn`. |
| `foreman_url` | `https://` plus `Foreman.spec.fqdn`, used by the proxy for template and callback requests. |

## Registration

The operator registers each proxy through the Foreman API with OAuth 1.0a:

- The `Foreman` controller generates `oauth-consumer-key` and `oauth-consumer-secret` in `<name>-secrets` and sets `oauth_active: true` and `oauth_map_users: false` in `settings.yaml`.
- The `SmartProxy` controller connects over HTTPS to the frontend through the in-cluster Service `https://<foreman>-web.<namespace>.svc`. It sends the public host as SNI and `Host` and signs the request for the public URL, because Foreman verifies the signature against that URL. It verifies the frontend certificate against `ca.crt` of the server certificate Secret, or against the system roots when the Secret has no `ca.crt`.
- It registers only after the proxy Deployment is available and the `Foreman` is `Ready`.
- It looks up the proxy by name, the name of the `SmartProxy` resource. It creates the proxy if it does not exist, or adopts the existing one, and records the ID in `status.foremanID`.
- When the URL, the generated configuration, or the certificate changes, it updates name and URL and refreshes the features, then records `status.registeredChecksum`. Foreman only reads the features on registration and refresh.
- If the proxy was removed in Foreman by hand, it registers it again.
- The finalizer `platform.theforeman.org/smart-proxy-registration` removes the proxy from Foreman when the resource is deleted. If the `Foreman` is gone or being deleted, the finalizer is removed without calling Foreman.

Registration errors set reason `RegistrationFailed` and are retried every 30 seconds.

## Profiles

| Profile | Features | Network | Privileges |
|---|---|---|---|
| `Provisioning` | `tftp`, `dhcp`, `dns`, `bmc`, `templates`, `httpBoot` | Multus | Dedicated, see [Provisioning](provisioning.md) |
| `Execution` | `remoteExecutionSSH`, `ansible` | Pod network with egress to hosts | None beyond the default. Not implemented: the features are accepted by the API but not configured |

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

A proxy outside the cluster cannot use cert-manager directly. The operator could export a certificate bundle for it, similar to the `auth-bundle` command of `foremanctl`.

Environments in which Foreman provisions the nodes of its own cluster must keep a proxy outside that cluster. A proxy inside the cluster cannot rebuild it after a failure.
