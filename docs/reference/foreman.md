# Foreman

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `fqdn` | string | Yes | | Primary hostname. Used for `foreman_url`, the client certificate CN, and the Frontend |
| `aliases` | []string | No | | Additional accepted hostnames |
| `image` | [image](index.md#image) | No | | Foreman image |
| `plugins` | []string | No | | Plugins to enable. Each plugin must be present in the image. Today the list only changes the migration hash, see [Plugins](../concepts/plugins.md) |
| `tls` | [tls](index.md#tls) | Yes | | Source of the client certificate Foreman presents to Smart Proxies |
| `database.cluster.name` | string | Yes | | CloudNativePG Cluster name. The operator reads the `uri` key of its credentials Secret |
| `database.cluster.credentialsSecretRef` | string | No | `<cluster>-app` | Overrides the credentials Secret |
| `valkey.host` | string | Yes | | Valkey host |
| `valkey.port` | integer | No | `6379` | Valkey port |
| `secretsRef` | string | No | `<name>-secrets` | Secret with `encryption-key`, `secret-key-base`, `admin-password`, `oauth-consumer-key`, and `oauth-consumer-secret`. Missing keys are generated |
| `web.replicas` | integer | No | `2` | Web pods |
| `web.resources` | ResourceRequirements | No | requests 200m CPU and 1Gi, limit 3Gi | Compute resources. Puma workers are derived from the memory limit |
| `dynflow.workerReplicas` | integer | No | `1` | General workers |
| `dynflow.hostsQueueReplicas` | integer | No | `0` | Hosts queue workers, used by Remote Execution |
| `dynflow.resources` | ResourceRequirements | No | requests 100m CPU and 512Mi, limit 1536Mi | Compute resources of all Dynflow workloads |

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | `Pending`, `Migrating`, `Progressing`, `Running`, or `Failed` |
| `appliedMigrationHash` | string | Hash of the image reference and the sorted plugin list the database was last migrated for |
| `conditions` | []Condition | See [Conditions](index.md#conditions) |

## Created objects

| Object | Name | Owner reference | Notes |
|---|---|---|---|
| Secret | `<name>-secrets` or `secretsRef` | No | Generated once; deleting the `Foreman` never deletes the encryption key |
| Secret | `<name>-settings` | Yes | `settings.yaml`, contains the OAuth consumer secret |
| ConfigMap | `<name>-dynflow` | Yes | Sidekiq configurations of the orchestrator, worker, and hosts queue |
| Certificate | `<name>-client` | Yes | Only with `tls.issuerRef`. CN and DNS name equal `fqdn`, client authentication usage |
| Service | `<name>-web` | Yes | Port 3000, used by the Frontend and the operator |
| Job | `<name>-migrate-<hash>` | Yes | First eight characters of the migration hash |
| Deployment | `<name>-web` | Yes | Puma, about 1.5 GiB memory limit per worker, 1 to 16 workers |
| Deployment | `<name>-dynflow-orchestrator` | Yes | Always one replica, strategy `Recreate` |
| Deployment | `<name>-dynflow-worker` | Yes | `dynflow.workerReplicas` |
| Deployment | `<name>-dynflow-hosts-queue` | Yes | `dynflow.hostsQueueReplicas` |

All Foreman pods disable service links. A Service named `foreman` would otherwise inject `FOREMAN_PORT`, which Puma reads as its listen port.

## Migration

The operator computes a hash over the image reference and the sorted plugin list. When it differs from `status.appliedMigrationHash`, the operator:

1. scales the web and all Dynflow Deployments to zero,
2. waits until no Foreman pods remain,
3. runs the Job `<name>-migrate-<hash>` with `db:migrate`, `db:seed`, and `upgrade:run`,
4. records the hash and rolls out the Deployments.

A failed Job sets the phase to `Failed` and the condition `Degraded` with reason `MigrationFailed`. Read the Job logs, fix the cause, and delete the Job to retry.

## Example

```yaml
--8<-- "config/samples/foreman.yaml"
```

The sample lists `foreman_discovery`, which the default Foreman image does not contain yet.
