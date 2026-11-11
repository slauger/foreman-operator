# Certificates

!!! note "Status"
    Implemented. Certificates from an internal CA issuer are verified on the E2E cluster for Foreman and the Frontend. The Let's Encrypt section is target design and not tested.

Foreman uses a static PKI, which cert-manager can provide. The operator has no certificate CRDs of its own. Each TLS consumer has a `tls` block with either `issuerRef` or `secretName`. With `issuerRef`, the operator creates a cert-manager `Certificate` with an RSA 4096 key, and the Secret has the same name as the Certificate.

| Purpose | Certificate | Issuer | Notes |
|---|---|---|---|
| Frontend server certificate | `<frontend>-server` | ACME, a corporate issuer, or the internal CA | DNS names are `spec.fqdn` and `spec.aliases` of the Foreman |
| Foreman client certificate | `<foreman>-client` | Internal CA | CN equals `spec.fqdn`, client authentication usage |
| Smart Proxy certificate | `<proxy>-tls` | Internal CA | CN is the host of `spec.url`, DNS names add the proxy Service names. Server and client authentication usage |
| Client verification at the Frontend | `<frontend>-client-ca` | Internal CA | A small client certificate, requested only to obtain the issuer CA in `ca.crt` |

Each pod trusts the `ca.crt` of its own Secret. Foreman, the Frontend client verification, and the Smart Proxies must therefore use the same internal CA.

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

The configuration checksum includes the certificate Secrets. Healthy resources are reconciled every 10 minutes, so after cert-manager renews a certificate, the operator rolls out the affected pods within that interval. Apache and the Smart Proxy read their certificates only at startup, which is why a rollout is needed.
