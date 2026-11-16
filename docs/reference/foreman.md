# Foreman

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `fqdn` | string | Yes | | Primary hostname. Used for `foreman_url`, the client certificate CN, and the frontend |
| `aliases` | []string | No | | Additional accepted hostnames |
| `image` | [image](index.md#image) | No | | Foreman image |
| `plugins` | []string | No | | Plugins to enable from the [catalog](../concepts/plugins.md#catalog). Dependencies are enabled with them, and a change runs a migration |
| `tls` | [tls](index.md#tls) | Yes | | Source of the client certificate Foreman presents to Smart Proxies |
| `database.cluster.name` | string | Yes | | CloudNativePG Cluster name. The operator reads the `uri` key of its credentials Secret |
| `database.cluster.credentialsSecretRef` | string | No | `<cluster>-app` | Overrides the credentials Secret |
| `valkey.host` | string | Yes | | Valkey host |
| `valkey.port` | integer | No | `6379` | Valkey port |
| `secretsRef` | string | No | `<name>-secrets` | Secret with `encryption-key`, `secret-key-base`, `admin-password`, `oauth-consumer-key`, and `oauth-consumer-secret`. Missing keys are generated |
| `web.replicas` | integer | No | `2` | Web pods. Each pod runs Puma and its own frontend container |
| `web.resources` | ResourceRequirements | No | requests 200m CPU and 1Gi, limit 3Gi | Compute resources. Puma workers are derived from the memory limit |
| `dynflow.workerReplicas` | integer | No | `1` | General workers |
| `dynflow.hostsQueueReplicas` | integer | No | `0` | Hosts queue workers, used by Remote Execution |
| `dynflow.resources` | ResourceRequirements | No | requests 100m CPU and 512Mi, limit 1536Mi | Compute resources of all Dynflow workloads |
| `frontend` | object | Yes | | Apache container in every web pod, see [`spec.frontend`](#specfrontend) |

### `spec.frontend`

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `image` | [image](index.md#image) | No | | Frontend image |
| `resources` | ResourceRequirements | No | requests 20m CPU and 64Mi, limit 256Mi | Compute resources of the `httpd` container |
| `tls.server` | [tls](index.md#tls) | Yes | | Certificate presented to clients |
| `tls.clientCA` | [tls](index.md#tls) | Yes | | CA that verifies client certificates. Only `ca.crt` of the Secret is used |
| `exposure.type` | string | No | `Auto` | `Auto`, `LoadBalancer`, `Route`, `TLSRoute`, or `ClusterIP` |
| `exposure.gatewayRef.name` | string | For `TLSRoute` | | Gateway name |
| `exposure.gatewayRef.namespace` | string | No | Foreman namespace | Gateway namespace |
| `exposure.gatewayRef.tlsSectionName` | string | No | | Listener with TLS passthrough for the TLSRoute |
| `exposure.gatewayRef.httpSectionName` | string | No | | Listener for the HTTPRoute used by installers. Without it, no HTTPRoute is created and an existing one is deleted |
| `exposure.annotations` | map | No | | Added to the Service `<name>-web` for `LoadBalancer` and `ClusterIP`, or to the Route for `Route` |

There is no separate replica count. `web.replicas` scales Puma and the frontend together, see [Traffic Flow](../concepts/traffic-flow.md#why-a-sidecar).

`Auto` resolves to `Route` when the cluster serves the OpenShift Route API, and to `LoadBalancer` otherwise. `Route` and `TLSRoute` fail with a reconcile error when the API is not installed.

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | `Pending`, `Migrating`, `Progressing`, `Running`, `Blocked`, `RollForwardRequired`, or `Failed` |
| `appliedMigrationHash` | string | Hash of the image reference and the sorted plugin list the database was last migrated for |
| `pendingMigrationHash` | string | Hash of a migration that passed the preflight but has not completed. Once its Job exists, the database may be partly migrated |
| `verifiedMigrationHash` | string | Hash of the last migration whose smoke checks passed |
| `foremanVersion` | string | Foreman version the database was last migrated to, read from the image by the preflight Job |
| `appliedPlugins` | []string | Plugins with dependencies the database was last migrated for |
| `conditions` | []Condition | See [Conditions](index.md#conditions). `Verified` reports the smoke checks after the last migration |

## Created objects

| Object | Name | Owner reference | Notes |
|---|---|---|---|
| Secret | `<name>-secrets` or `secretsRef` | No | Generated once; deleting the `Foreman` never deletes the encryption key |
| Secret | `<name>-settings` | Yes | `settings.yaml`, contains the OAuth consumer secret |
| ConfigMap | `<name>-dynflow` | Yes | Sidekiq configurations of the orchestrator, worker, and hosts queue |
| Certificate | `<name>-client` | Yes | Only with `tls.issuerRef`. CN and DNS name equal `fqdn`, client authentication usage |
| Certificate | `<name>-server` | Yes | Only with `frontend.tls.server.issuerRef`. DNS names are `fqdn` and `aliases` |
| Certificate | `<name>-client-ca` | Yes | Only with `frontend.tls.clientCA.issuerRef`. A small client certificate, requested only to obtain the issuer CA in `ca.crt` |
| ConfigMap | `<name>-frontend` | Yes | Apache configuration generated by the operator |
| Service | `<name>-web` | Yes | Ports 443 (HTTPS) and 80 (HTTP) of the frontend containers. Type `LoadBalancer` for exposure `LoadBalancer`, otherwise `ClusterIP`. Used by clients and the operator |
| Route | `<name>` | Yes | Exposure `Route`, or `Auto` on OpenShift. Host is `fqdn`, termination `passthrough`, target `<name>-web` |
| TLSRoute | `<name>` | Yes | Exposure `TLSRoute`. Hostnames are `fqdn` and `aliases`, backend `<name>-web` port 443 |
| HTTPRoute | `<name>` | Yes | Exposure `TLSRoute` with `httpSectionName` set. Backend `<name>-web` port 80 |
| Job | `<name>-preflight-<hash>` | Yes | Reads the Foreman version from `/usr/share/foreman/VERSION` of the new image into its termination message |
| Job | `<name>-migrate-<hash>` | Yes | First eight characters of the migration hash. The annotation `platform.theforeman.org/foreman-version` records the version it migrates to |
| CronJob | `<name>-cron-hourly`, `-daily`, `-weekly`, `-monthly` | Yes | `bin/rails cron:<period>` on the schedules of `/etc/cron.d/foreman` in the Foreman RPM: `30 * * * *`, `0 2 * * *`, `0 5 * * 0`, `0 3 1 * *`, in the time zone of the kube-controller-manager. Runs never overlap, a run missed by more than ten minutes is skipped. Suspended while a migration stops the writers |
| Deployment | `<name>-web` | Yes | Containers `foreman` (Puma, about 1.5 GiB memory limit per worker, 1 to 16 workers) and `httpd` (Apache on ports 8443 and 8080) |
| Deployment | `<name>-dynflow-orchestrator` | Yes | Always one replica, strategy `Recreate` |
| Deployment | `<name>-dynflow-worker` | Yes | `dynflow.workerReplicas` |
| Deployment | `<name>-dynflow-hosts-queue` | Yes | `dynflow.hostsQueueReplicas` |

All Foreman pods disable service links. A Service named `foreman` would otherwise inject `FOREMAN_PORT`, which Puma reads as its listen port.

In the web pods, Puma binds only to the Unix socket `/run/puma/puma.sock` in a shared `emptyDir` (`FOREMAN_BIND=unix:///run/puma/puma.sock?umask=0007`) and has no TCP listener. Its readiness and liveness probes are exec probes that run `curl` against `/api/v2/ping` over the socket. A change to the frontend configuration or its certificates rolls only the web pods, not Dynflow.

## Migration

The operator computes a hash over the image reference and the sorted plugin list. When it differs from `status.appliedMigrationHash`, it runs the preflight, then the migration, and the smoke checks after the rollout. See [Upgrades](../concepts/upgrades.md) for the checks and how to recover.

| Situation | Phase | Condition and reason |
|---|---|---|
| Preflight Job reads the version | `Progressing` | `Progressing`, `Preflight` |
| Unsupported version path | `Blocked` | `Degraded`, `UnsupportedUpgradePath` |
| A plugin that schedules tasks would be disabled | `Blocked` | `Degraded`, `PluginRemovalRefused` |
| Tasks are planning, running, or paused | `Progressing` | `Progressing`, `WaitingForTasks` |
| Writers stopping, migration running | `Migrating` | `Progressing`, `StoppingWriters` or `Migrating` |
| Migration Job failed | `RollForwardRequired` | `Degraded`, `MigrationFailed` |
| Previous image requested after the migration Job started | `RollForwardRequired` | `Degraded` and `Ready`, `RollForwardRequired` |
| Rolled out, smoke checks pending | `Running` | `Verified` is `False`, `Verifying` or `VerificationFailed` |
| Smoke checks passed | `Running` | `Verified`, `SmokeChecksPassed` |

While the preflight runs or blocks, the previous version keeps running.

## Example

```yaml
--8<-- "config/samples/foreman.yaml"
```

The sample lists `foreman_discovery`, which the default Foreman image does not contain yet.
