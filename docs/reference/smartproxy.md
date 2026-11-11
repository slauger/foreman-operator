# SmartProxy

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `foremanRef` | string | Yes | | Foreman this proxy registers with, in the same namespace |
| `profile` | string | Yes | | `Provisioning` or `Execution`. Only `Provisioning` is implemented |
| `url` | string | Yes | | Address registered in Foreman, must start with `https://`. Its host is the certificate CN, its port the HTTPS port of the proxy |
| `image` | [image](index.md#image) | No | | Smart Proxy image, also used for the dnsmasq container |
| `tls` | [tls](index.md#tls) | Yes | | Source of the server and client certificate |
| `networks[].attachment` | string | Yes | | NetworkAttachmentDefinition name, in the same namespace |
| `networks[].address` | string | Yes | | Static address in CIDR notation |
| `features` | object | Yes | | See below |
| `storage.size` | Quantity | Yes, if `storage` is set | | Volume for the TFTP root. Without `storage`, an `emptyDir` is used |
| `storage.storageClass` | string | No | | Empty uses the cluster default |
| `resources` | ResourceRequirements | No | requests 50m CPU and 256Mi, limit 512Mi | Compute resources of the proxy container |

Networks are attached as `net1` to `netN` in list order. Only the first network gets its static address, assigned by the dnsmasq container, and dnsmasq serves only `net1`. The NetworkAttachmentDefinition can therefore use `"ipam": {}`.

## Features

| Field | Required | Description |
|---|---|---|
| `tftp.serverName` | No | Overrides the announced TFTP server name. Defaults to the address of the first network |
| `tftp.bootFiles` | No | [image](index.md#image) whose boot loaders an init container copies into the TFTP root. Defaults to the `boot-files` image |
| `templates.url` | Yes | Address hosts use to fetch templates, usually `http://<address>:8000` |
| `dhcp.provider` | Yes | `dnsmasq` or `infoblox`. Only `dnsmasq` is in the image |
| `dhcp.subnets[].network` | Yes | Subnet in CIDR notation, IPv4 only |
| `dhcp.subnets[].range` | For `dnsmasq` | Dynamic range as `first-last` |
| `dhcp.subnets[].gateway` | No | Announced as the default router (option 3). Without it, no router is announced |
| `dhcp.subnets[].dnsServers` | No | Announced DNS servers (option 6) |
| `dns.provider` | No | `dns_nsupdate` (default) or `dns_nsupdate_gss`. Only `dns_nsupdate` with a TSIG key is wired up |
| `dns.server` | Yes | DNS server to update |
| `dns.keySecretRef` | No | Secret with the TSIG key under `key` |
| `bmc.defaultProvider` | No | `ipmitool`, `freeipmi`, `redfish`, `shell`, or `ssh`. Not usable yet, see [Provisioning](../concepts/provisioning.md#dns-and-bmc) |
| `httpBoot` | No | Enables UEFI HTTP Boot from the TFTP root on port 8000 |
| `remoteExecutionSSH.sshKeySecretRef` | Yes | Secret with `id_rsa` and `id_rsa.pub`. Accepted by the API, not implemented |
| `ansible` | No | Enables Ansible. Accepted by the API, not implemented |

## Validation

- `Execution` does not support `dhcp`, `tftp`, or `dns`.
- `Provisioning` does not support `remoteExecutionSSH` or `ansible`.
- The `dnsmasq` provider requires at least one network.
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
