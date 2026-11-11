# foreman-operator

Helm chart for foreman-operator, an experimental Kubernetes operator for Foreman and Smart Proxy. The operator reconciles the `Foreman`, `Frontend`, and `SmartProxy` custom resources of the API group `platform.theforeman.org/v1alpha1`.

> **Experimental.** This is an unofficial, unsupported prototype. It is not affiliated with or endorsed by the Foreman project. Do not use it in production.

## Install

The chart is not published to a registry yet. Install it from a checkout of the repository. Helm installs the CRDs in `crds/` only on the first install and never upgrades them, so apply them explicitly before every install or upgrade:

```bash
kubectl apply --server-side -f charts/foreman-operator/crds/

helm upgrade --install foreman-operator charts/foreman-operator \
  --namespace foreman-operator \
  --create-namespace \
  --set image.tag=develop
```

`image.tag` defaults to the chart `appVersion` `0.0.0`. No image with that tag is published yet, so set it to `develop` or a branch tag.

To test a feature branch, set both the operator image tag and the default tag of the component images:

```bash
helm upgrade --install foreman-operator charts/foreman-operator \
  --namespace foreman-operator \
  --set image.tag=feat-example \
  --set operator.defaultImageTag=feat-example
```

cert-manager, CloudNativePG, and Valkey are prerequisites of the managed resources, not of the chart. See the [Getting Started](../../docs/guides/getting-started.md) guide.

## RBAC

The RBAC rules are rendered from `files/rules.yaml`, which `make manifests` generates from the controllers. Without `operator.watchNamespace`, the chart creates a ClusterRole and ClusterRoleBinding. With `operator.watchNamespace`, it creates a Role and RoleBinding in that namespace. With `operator.leaderElection`, it adds a Role for leases and events.

## Values

| Key | Type | Default | Description |
|---|---|---|---|
| `image.repository` | string | `ghcr.io/slauger/foreman-operator` | Operator image repository |
| `image.tag` | string | `""` | Operator image tag. Empty uses the chart `appVersion` |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy |
| `imagePullSecrets` | list | `[]` | Image pull secrets for private registries |
| `nameOverride` | string | `""` | Override the chart name |
| `fullnameOverride` | string | `""` | Override the fully qualified app name |
| `operator.defaultImageTag` | string | `develop` | Tag of the Foreman, Frontend, Smart Proxy, and boot-files images when a resource names neither tag nor digest |
| `operator.watchNamespace` | string | `""` | Restrict the operator to one namespace. Empty watches all namespaces |
| `operator.leaderElection` | bool | `true` | Enable leader election. Required for more than one replica |
| `operator.logLevel` | string | `info` | Log level: `debug`, `info`, or `error` |
| `operator.metricsSecure` | bool | `false` | Serve metrics over HTTPS with authentication and authorization |
| `replicaCount` | int | `1` | Number of operator replicas |
| `podAnnotations` | object | `{}` | Additional pod annotations |
| `podLabels` | object | `{}` | Additional pod labels |
| `extraEnv` | object | `{}` | Additional environment variables as key-value pairs |
| `extraEnvSecret` | string | `""` | Existing Secret whose keys become environment variables |
| `nodeSelector` | object | `{}` | Node selector for pod assignment |
| `tolerations` | list | `[]` | Tolerations for pod assignment |
| `affinity` | object | `{}` | Affinity and anti-affinity rules |
| `topologySpreadConstraints` | list | `[]` | Topology spread constraints |
| `priorityClassName` | string | `""` | Priority class name |
| `resources` | object | requests 10m CPU and 64Mi, limit 256Mi | Container resource requests and limits |
| `updateStrategy` | object | `{}` | Deployment update strategy |
| `terminationGracePeriodSeconds` | int | `10` | Termination grace period |
| `podDisruptionBudget.enabled` | bool | `false` | Enable a PodDisruptionBudget |
| `podDisruptionBudget.minAvailable` | int or string | `""` | Minimum available pods |
| `podDisruptionBudget.maxUnavailable` | int or string | `1` | Maximum unavailable pods |
| `serviceAccount.create` | bool | `true` | Create a service account |
| `serviceAccount.name` | string | `""` | Service account name. Generated if empty |
| `serviceAccount.annotations` | object | `{}` | Service account annotations |
| `livenessProbe` | object | HTTP `/healthz` on port `probes` | Liveness probe configuration |
| `readinessProbe` | object | HTTP `/readyz` on port `probes` | Readiness probe configuration |
| `startupProbe` | object | `{}` | Startup probe configuration |
| `extraVolumes` | list | `[]` | Additional volumes |
| `extraVolumeMounts` | list | `[]` | Additional volume mounts |
| `extraContainers` | list | `[]` | Additional sidecar containers |
| `extraInitContainers` | list | `[]` | Additional init containers |

## Tests

```bash
make helm-lint
make helm-unittest
```

## License

Apache License 2.0.
