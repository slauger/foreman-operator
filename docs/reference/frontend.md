# Frontend

## Spec

| Field | Type | Required | Default | Description |
|---|---|---|---|---|
| `foremanRef` | string | Yes | | Foreman served by this Frontend |
| `replicas` | integer | No | `2` | Frontend pods |
| `image` | [image](index.md#image) | No | | Frontend image |
| `tls.server` | [tls](index.md#tls) | Yes | | Certificate presented to clients |
| `tls.clientCA` | [tls](index.md#tls) | Yes | | CA that verifies client certificates |
| `exposure.type` | string | No | `Auto` | `Auto`, `LoadBalancer`, `Route`, `TLSRoute`, or `ClusterIP` |
| `exposure.gatewayRef.name` | string | For `TLSRoute` | | Gateway name |
| `exposure.gatewayRef.namespace` | string | No | Frontend namespace | Gateway namespace |
| `exposure.gatewayRef.tlsSectionName` | string | No | | Listener with TLS passthrough |
| `exposure.gatewayRef.httpSectionName` | string | No | | Listener for installers |
| `exposure.annotations` | map | No | | Added to the created Service or Route |

## Status

| Field | Type | Description |
|---|---|---|
| `observedGeneration` | integer | Last processed generation |
| `phase` | string | Current lifecycle phase |
| `conditions` | []Condition | See [Conditions](index.md#conditions) |

## Example

```yaml
--8<-- "config/samples/frontend.yaml"
```
