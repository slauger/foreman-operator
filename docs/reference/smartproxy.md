# SmartProxy

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `foremanRef` | string | Yes | | Foreman this proxy registers with |
| `profile` | string | Yes | | `Provisioning` or `Execution` |
| `url` | string | Yes | | Address registered in Foreman, must start with `https://` |
| `image` | [image](index.md#image) | No | | Smart Proxy image |
| `tls` | [tls](index.md#tls) | Yes | | Source of the server and client certificates |
| `oauthSecretRef` | string | Yes | | Secret with `consumer-key` and `consumer-secret` |
| `networks[].attachment` | string | Yes | | NetworkAttachmentDefinition name |
| `networks[].address` | string | Yes | | Static address in CIDR notation |
| `features` | object | Yes | | See below |
| `storage.size` | Quantity | Yes | | Volume for leases and boot files |
| `storage.storageClass` | string | No | | Empty uses the cluster default |
| `resources` | ResourceRequirements | No | | Compute resources of the proxy container |

## Features

| Field | Required | Description |
|---|---|---|
| `tftp.serverName` | No | Overrides the announced TFTP server name |
| `templates.url` | Yes | Address hosts use to fetch templates |
| `dhcp.provider` | Yes | `dnsmasq` or `infoblox` |
| `dhcp.subnets[].network` | Yes | Subnet in CIDR notation |
| `dhcp.subnets[].range` | No | Dynamic range as `first-last` |
| `dns.provider` | No | `dns_nsupdate` (default) or `dns_nsupdate_gss` |
| `dns.server` | Yes | DNS server to update |
| `dns.keySecretRef` | No | Secret with the TSIG key under `key` |
| `bmc.defaultProvider` | No | `ipmitool`, `freeipmi`, `redfish`, `shell`, or `ssh` |
| `httpBoot` | No | Enables UEFI HTTP Boot |
| `remoteExecutionSSH.sshKeySecretRef` | Yes | Secret with `id_rsa` and `id_rsa.pub` |
| `ansible` | No | Enables Ansible |

## Validation

- `Execution` does not support `dhcp`, `tftp`, or `dns`.
- `Provisioning` does not support `remoteExecutionSSH` or `ansible`.
- The `dnsmasq` provider requires at least one network.
- The `dnsmasq` provider supports exactly one subnet per `SmartProxy`.

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | Current lifecycle phase |
| `foremanID` | integer | Identifier assigned by Foreman |
| `conditions` | []Condition | See [Conditions](index.md#conditions) |

## Example

```yaml
--8<-- "config/samples/smartproxy-provisioning.yaml"
```
