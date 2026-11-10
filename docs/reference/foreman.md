# Foreman

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `fqdn` | string | Yes | | Primary hostname |
| `aliases` | []string | No | | Additional accepted hostnames |
| `image` | [image](index.md#image) | No | | Foreman image |
| `plugins` | []string | No | | Plugins to enable |
| `tls` | [tls](index.md#tls) | Yes | | Source of the internal certificates |
| `database.cluster.name` | string | Yes | | CloudNativePG Cluster name |
| `database.cluster.credentialsSecretRef` | string | No | `<name>-app` | Overrides the credentials Secret |
| `valkey.host` | string | Yes | | Valkey host |
| `valkey.port` | integer | No | `6379` | Valkey port |
| `encryptionKeySecretRef` | string | Yes | | Secret with `encryption-key` |
| `web.replicas` | integer | No | `2` | Web pods |
| `web.resources` | ResourceRequirements | No | | Compute resources |
| `dynflow.workerReplicas` | integer | No | `1` | General workers |
| `dynflow.hostsQueueReplicas` | integer | No | `0` | Hosts queue workers, used by Remote Execution |
| `dynflow.resources` | ResourceRequirements | No | | Compute resources of workers |

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | `Pending`, `Migrating`, `Progressing`, `Running`, or `Failed` |
| `conditions` | []Condition | See [Conditions](index.md#conditions) |

## Example

```yaml
--8<-- "config/samples/foreman.yaml"
```
