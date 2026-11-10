# Katello

!!! note "Status"
    A later stage. Nothing here is implemented. It is recorded so that the base design does not block it.

`Candlepin` and `Pulp` become separate kinds. Both use CloudNativePG and the internal PKI.

## Candlepin

- One replica with `Recreate`. Candlepin migrates its schema on startup, so an upgrade starts it and waits for health.
- It needs a CA certificate and private key. `foremanctl` mounts the default CA key. The CA must therefore be an issuer whose key is available in a Secret.
- A separate CA for Candlepin would limit what its key can sign. `Frontend` would then trust both CAs. This is untested.
- Katello expects `https://candlepin:23443`, so the Service and SAN must match.
- Secrets are mounted with group 53, which needs `supplementalGroups` under a random UID.

## Pulp

- `pulp-api`, `pulp-content`, and `pulp-worker`, plus a migration Job.
- Katello reaches Pulp through a Smart Proxy registration named `<fqdn>-pulp`. The `Pulp` controller performs it.
- `Frontend` maps a client certificate with CN equal to the FQDN to `REMOTE-USER: admin`. NetworkPolicy must restrict Pulp accordingly.
- S3-compatible storage is preferred. Import and export paths stay on a filesystem.

## Impact on the base

- `Frontend` adds `/pulp`, `/rhsm`, and `/pub/katello-rhsm-consumer` rules.
- The upgrade fence includes Pulp and Candlepin.
- Valkey adds database 8 for Pulp.
- More secrets must be preserved: the Candlepin OAuth secret, the Pulp `SECRET_KEY`, and `database_fields.symmetric.key`.

## Content proxies

A content mirror is Pulp in mirror mode with a frontend and a Smart Proxy. `container_gateway` needs its own PostgreSQL database.
