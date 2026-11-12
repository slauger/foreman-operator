# SmartProxy

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `foremanRef` | string | Yes | | Foreman this proxy registers with, in the same namespace |
| `profile` | string | Yes | | `Provisioning` or `Execution` |
| `url` | string | Yes | | Address registered in Foreman, must start with `https://`. Its host is the certificate CN, its port the HTTPS port of the proxy |
| `image` | [image](index.md#image) | No | | Smart Proxy image, also used for the dnsmasq container |
| `tls` | [tls](index.md#tls) | Yes | | Source of the server and client certificate |
| `networks` | list | For `tftp` and `dnsmasq` | | At most one network |
| `networks[].attachment` | string | Yes | | NetworkAttachmentDefinition name, in the same namespace |
| `networks[].address` | string | Yes | | Static address in CIDR notation |
| `features` | object | Yes | | See below |
| `storage.size` | Quantity | Yes, if `storage` is set | | Volume for the TFTP root and the DHCP reservations and leases. Without `storage`, an `emptyDir` is used |
| `storage.storageClass` | string | No | | Empty uses the cluster default |
| `resources` | ResourceRequirements | No | requests 50m CPU and 256Mi, limit 512Mi | Compute resources of the proxy container |

The network is attached as `net1`. The dnsmasq container assigns its static address and serves only `net1`, so the NetworkAttachmentDefinition can use `"ipam": {}`.

## Features

| Field | Required | Description |
|---|---|---|
| `tftp.serverName` | No | Overrides the announced TFTP server name. Defaults to the address of the first network |
| `tftp.bootFiles` | No | [image](index.md#image) whose boot loaders an init container copies into the TFTP root. Defaults to the `boot-files` image |
| `templates.url` | Yes | Address hosts use to fetch templates, usually `http://<address>:8000` |
| `dhcp.provider` | Yes | `dnsmasq`, the only provider in the image |
| `dhcp.subnets[].network` | Yes | Subnet in CIDR notation, IPv4 only |
| `dhcp.subnets[].range` | For `dnsmasq` | Dynamic range as `first-last` |
| `dhcp.subnets[].gateway` | No | Announced as the default router (option 3). Without it, no router is announced |
| `dhcp.subnets[].dnsServers` | No | Announced DNS servers (option 6) |
| `dns.provider` | No | `dns_nsupdate` (default and only value) |
| `dns.server` | Yes | DNS server to update |
| `dns.keySecretRef` | No | Secret with the TSIG key under `key` |
| `bmc.defaultProvider` | No | `ipmitool` (default), `redfish`, `shell`, or `ssh`. See [Provisioning](../concepts/provisioning.md#dns-and-bmc) |
| `httpBoot` | No | Enables UEFI HTTP Boot from the TFTP root on port 8000 |
| `remoteExecutionSSH.sshKeySecretRef` | Yes | Secret with `id_rsa` and `id_rsa.pub`. Enables Dynflow and Remote Execution over SSH, see [Smart Proxy](../concepts/smart-proxy.md#execution-profile) |
| `ansible` | No | Enables Ansible. Requires `remoteExecutionSSH` |

## Validation

- `Execution` does not support `dhcp`, `tftp`, or `dns`.
- `Provisioning` does not support `remoteExecutionSSH` or `ansible`.
- `ansible` requires `remoteExecutionSSH`.
- At most one network is supported.
- The `dnsmasq` provider and `tftp` require a network.
- The `dnsmasq` provider supports exactly one subnet per `SmartProxy`.
- The `dnsmasq` provider requires a `range`.

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | `Pending`, `Progressing`, `Running`, or `Failed` |
| `foremanID` | integer | Identifier assigned by Foreman |
| `registeredChecksum` | string | URL, configuration, and certificate whose features were last refreshed in Foreman |
| `conditions` | []Condition | See [Conditions](index.md#conditions) |

## Created objects

| Object | Name | Owner reference | Notes |
|---|---|---|---|
| Certificate | `<name>-tls` | Yes | Only with `tls.issuerRef`. Server and client usage. CN is the host of `url`, DNS names add the Service names |
| ConfigMap | `<name>` | Yes | `settings.yml`, module settings, and `dnsmasq.conf` |
| PersistentVolumeClaim | `<name>-data` | No | Only with `storage`. Survives deletion of the `SmartProxy` |
| Deployment | `<name>` | Yes | One replica, strategy `Recreate` |
| Service | `<name>` | Yes | HTTPS port of `url` |

The finalizer `platform.theforeman.org/smart-proxy-registration` removes the proxy from Foreman before the resource is deleted. If the Foreman still exists but cannot be reached, the finalizer stays until it can.

## Example

```yaml
--8<-- "config/samples/smartproxy-provisioning.yaml"
```

The namespace needs the Pod Security level `privileged`, see [Provisioning](../concepts/provisioning.md#pod-layout).
