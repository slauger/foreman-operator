# API Reference

All resources use the API group `platform.theforeman.org/v1alpha1`.

| Kind | Reference |
|---|---|
| Foreman | [Foreman](foreman.md) |
| Frontend | [Frontend](frontend.md) |
| SmartProxy | [SmartProxy](smartproxy.md) |

!!! note
    The generated schemas are in `config/crd/bases`. CEL validation rules are tested against an API server with envtest.

## Common types

### `tls`

Exactly one field must be set.

| Field | Type | Description |
|---|---|---|
| `issuerRef.name` | string | Name of a cert-manager issuer |
| `issuerRef.kind` | string | `Issuer` (default) or `ClusterIssuer` |
| `secretName` | string | Existing Secret with `tls.crt`, `tls.key`, and `ca.crt` |

### `image`

| Field | Type | Description |
|---|---|---|
| `repository` | string | Image repository |
| `tag` | string | Image tag |
| `digest` | string | Pins the image, takes precedence over `tag` |
| `pullPolicy` | string | `Always`, `IfNotPresent` (default), or `Never` |

## Conditions

| Type | Meaning |
|---|---|
| `Ready` | The resource is fully reconciled and serving |
| `Progressing` | A change is being rolled out |
| `Degraded` | The resource runs with reduced function |
| `Paused` | Reconciliation is suspended by annotation |
