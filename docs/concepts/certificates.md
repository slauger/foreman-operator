# Certificates

!!! note "Status"
    Target design, not tested.

Foreman uses a static PKI, which cert-manager can provide. The operator has no certificate CRDs of its own. Each TLS consumer has a `tls` block with either `issuerRef` or `secretName`.

| Purpose | Issuer | Notes |
|---|---|---|
| Frontend server certificate | ACME or a corporate issuer | Independent of client verification |
| Foreman client certificate | Internal CA | CN equals `spec.fqdn`, client authentication usage |
| Smart Proxy certificates | Internal CA | SAN matches `spec.url` |
| Client verification at the Frontend | Internal CA | |

The internal PKI is required in the base deployment. Foreman and Smart Proxies authenticate each other with client certificates, and public CAs are phasing out the client authentication usage.

## Let's Encrypt

- **DNS-01** works without inbound access from the internet. It needs a cert-manager solver for the DNS provider.
- **HTTP-01** needs port 80 reachable from the internet, and `Frontend` must route `/.well-known/acme-challenge/` to the solver.
- **ExternalDNS** can publish address records for `spec.fqdn` and proxy URLs. It does not answer ACME challenges.
- A proxy that reaches Foreman through a public certificate needs the public roots in `foreman_ssl_ca`.

## Internal CA

The internal CA is a cert-manager `Certificate` with `isCA: true`, consumed by a CA `Issuer`.

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: foreman-internal-ca
spec:
  isCA: true
  commonName: foreman-internal-ca
  secretName: foreman-internal-ca
  duration: 175200h
  privateKey:
    algorithm: RSA
    size: 4096
    rotationPolicy: Never
  issuerRef:
    name: selfsigned
    kind: ClusterIssuer
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: foreman-internal-ca
spec:
  ca:
    secretName: foreman-internal-ca
```

Set `rotationPolicy: Never`. A new CA key would break trust everywhere, and cert-manager rotates private keys by default since version 1.18.

## Rollouts

Renewals roll pods through the configuration hash. Internal certificates should have long lifetimes to limit restarts.
