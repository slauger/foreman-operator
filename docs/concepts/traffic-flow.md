# Traffic Flow

!!! note "Status"
    Target design, not tested.

## Why a Frontend tier

Foreman uses client certificates as part of its protocol. The frontend verifies them optionally and forwards the result as `SSL-CLIENT-S-DN`, `SSL-CLIENT-CERT`, and `SSL-CLIENT-VERIFY`.

Some ingress controllers can verify and forward client certificates. The OpenShift Router does not provide this in the form Foreman expects. A portable design therefore uses **TLS passthrough** to a `Frontend` tier in the cluster, with one set of path rules on every platform.

## Exposure

| `exposure.type` | Result |
|---|---|
| `Auto` | Route on OpenShift, LoadBalancer elsewhere |
| `LoadBalancer` | Service of type LoadBalancer |
| `Route` | OpenShift Route with `termination: passthrough` |
| `TLSRoute` | Gateway API TLSRoute |
| `ClusterIP` | Service only |

## What the Frontend serves

- HTTPS for users, the API, and proxies
- HTTP on port 80 for `/unattended` requests from installers
- `/pub` content, such as the CA certificate
- External authentication, where used

## Trust boundary

Foreman trusts the forwarded identity headers. Therefore:

- A NetworkPolicy limits Foreman web to traffic from `Frontend` and known components.
- `Frontend` removes these headers from client requests.
- `trusted_proxies` in Foreman lists the Frontend pods.

Foreman 5.0 rejects undeclared `Host` headers, so `spec.fqdn` and `spec.aliases` must be complete.

## Licensing

The `foremanctl` vhost templates are GPL-3.0. The `Frontend` configuration is written independently from the described behavior.
