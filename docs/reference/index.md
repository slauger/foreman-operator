# API Reference

All resources use the API group `platform.theforeman.org/v1alpha1`.

| Kind | Reference |
|---|---|
| Foreman | [Foreman](foreman.md) |
| SmartProxy | [SmartProxy](smartproxy.md) |

!!! note
    The generated schemas are in `config/crd/bases` and are copied to `charts/foreman-operator/crds`. CEL validation rules are tested against an API server with envtest.

## Common types

### `tls`

Exactly one field must be set.

| Field | Type | Description |
|---|---|---|
| `issuerRef.name` | string | Name of a cert-manager issuer |
| `issuerRef.kind` | string | `Issuer` (default) or `ClusterIssuer` |
| `secretName` | string | Existing Secret with `tls.crt`, `tls.key`, and `ca.crt` |

With `issuerRef`, the operator creates a cert-manager `Certificate` whose Secret has the same name as the Certificate. The names are listed on each kind's page. With `secretName`, the operator uses the Secret as it is and does not check its content.

### `image`

| Field | Type | Description |
|---|---|---|
| `repository` | string | Image repository. Empty uses the default image of the component |
| `tag` | string | Image tag. Empty uses the operator flag `--default-image-tag` |
| `digest` | string | Pins the image as `sha256:<64 hex>`, takes precedence over `tag` |
| `pullPolicy` | string | `Always`, `IfNotPresent` (default), or `Never` |

Default repositories:

| Component | Repository |
|---|---|
| Foreman | `ghcr.io/slauger/foreman-operator-foreman` |
| Frontend (`spec.frontend.image`) | `ghcr.io/slauger/foreman-operator-frontend` |
| Smart Proxy | `ghcr.io/slauger/foreman-operator-smart-proxy` |
| Boot files | `ghcr.io/slauger/foreman-operator-boot-files` |

## Phases

| Phase | Meaning |
|---|---|
| `Pending` | Waiting for a dependency, such as the database Secret or the referenced Foreman |
| `Migrating` | `Foreman` only: writers are stopped or the migration Job runs |
| `Blocked` | `Foreman` only: the upgrade preflight refused the change, the previous version keeps running |
| `RollForwardRequired` | `Foreman` only: a migration failed or the previous image was requested after a migration started. Foreman stays stopped |
| `Progressing` | Workloads are rolling out, or a Smart Proxy registration is retried |
| `Running` | Fully reconciled. Every pod of the web and orchestrator workloads, or of the proxy, runs the current template |
| `Failed` | A reconcile error |

## Conditions

| Type | Meaning |
|---|---|
| `Ready` | The resource is fully reconciled and serving |
| `Progressing` | A change is being rolled out |
| `Degraded` | The resource runs with reduced function or an upgrade is blocked. Reasons `MigrationFailed`, `RollForwardRequired`, `UnsupportedUpgradePath`, `PluginRemovalRefused`, and `PreflightFailed` |
| `Verified` | `Foreman` only: the smoke checks after the last migration passed, see [Upgrades](../concepts/upgrades.md#verify) |
| `Paused` | Reconciliation is suspended by annotation |

Reasons used on `Ready` include `Reconciled`, `ReconcileError`, `WaitingForDatabase`, `WaitingForForeman`, `Migrating`, `RollingOut`, `RollForwardRequired`, and `RegistrationFailed`. See [Foreman](foreman.md#migration) for the reasons during a migration.

## Annotations

| Annotation | Effect |
|---|---|
| `platform.theforeman.org/paused: "true"` | Suspends reconciliation of this object and sets `Paused` to true. Child objects are left as they are |
| `platform.theforeman.org/skip-upgrade-checks: "true"` | `Foreman` only: skips the upgrade preflight checks and the roll forward guard. For recovery only, see [Upgrades](../concepts/upgrades.md#overriding-the-checks) |
