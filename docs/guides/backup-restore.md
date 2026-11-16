# Backup and Restore

!!! note "Status"
    The restore order below is verified by the restore drill `tests/e2e/restore.sh` on the E2E cluster, with a logical dump of the database. The operator has no `Backup` kind; backups use CloudNativePG and plain Kubernetes Secrets.

## What to back up

| Data | Where | Without it |
|---|---|---|
| Foreman database | CloudNativePG `Cluster`, for example `foreman-db` | Everything is lost |
| `<name>-secrets` | Secret of the `Foreman`, or `spec.secretsRef` | `ENCRYPTION_KEY` decrypts compute resource, LDAP, and other encrypted columns. With a new key, these values cannot be read. `SECRET_KEY_BASE` invalidates sessions, the OAuth consumer the registration of the proxies |
| Internal CA | Secret of the cert-manager issuer, for example `foreman-internal-ca` | A new CA issues new certificates, so clients and hosts that trust the old CA fail |
| Remote Execution SSH keys | `remoteExecutionSSH.sshKeySecretRef` of `Execution` proxies | Hosts with the old public key reject jobs |
| TSIG and other referenced Secrets | `dns.keySecretRef` and similar | Features of the proxy fail until recreated |

The `Foreman` and `SmartProxy` resources themselves belong in Git. The TFTP root on the proxy volume is rebuilt from the boot files image, and the DHCP state from Foreman.

`<name>-secrets` has no owner reference, so deleting the `Foreman` keeps it. Store a copy outside the cluster anyway, together with the database backup it belongs to.

## Database backups

Use the backup support of CloudNativePG for production: continuous backups to an object store with the Barman Cloud plugin, or volume snapshots. Both allow a point-in-time recovery into a new `Cluster` through `bootstrap.recovery`. See the [CloudNativePG documentation](https://cloudnative-pg.io/documentation/current/backup/).

A logical dump works for smaller installations and is what the restore drill uses, since the E2E cluster has no object store:

```bash
kubectl -n foreman exec foreman-db-1 -c postgres -- pg_dump -Fc -d foreman > foreman.dump
kubectl -n foreman get secret foreman-secrets -o yaml > foreman-secrets.yaml
```

Take the dump while Foreman runs or after scaling it down. A dump during a migration is not consistent with either version.

## Restore order

Restore with the same Foreman version as the backup. The operator treats a `Foreman` without status as a new installation and skips the upgrade preflight, so a restore into a newer image upgrades the database without the version path check.

1. Restore `<name>-secrets`, the internal CA, and the other referenced Secrets **before** creating the `Foreman`. Otherwise the operator generates new secrets, and the encrypted columns become unreadable.
2. Restore the database into a new CloudNativePG `Cluster`, through `bootstrap.recovery` from a backup, or into an empty cluster with `pg_restore`:

    ```bash
    kubectl -n foreman exec -i foreman-db-1 -c postgres -- \
      pg_restore -d foreman --no-owner --role=app --exit-on-error < foreman.dump
    ```

3. Apply the `Foreman`. The operator runs the migration Job, which finds the schema up to date and seeds in full, then rolls out the workloads and verifies them.
4. Apply the `SmartProxy` resources. A proxy finds its registration in the restored database by name and adopts it, so its identifier in Foreman stays the same.

## Restore drill

`tests/e2e/restore.sh` installs `IMAGE_TAG` in an empty namespace with a Foreman, an `Execution` proxy, a domain, and an LDAP source with an encrypted password. It dumps the database and saves `foreman-secrets`, then deletes the `Foreman`, the proxy, the database cluster, and the Secret, and restores them in the order above. It checks that Foreman runs and is verified, the domain exists, the LDAP password can be decrypted, and the proxy kept its identifier.

```bash
IMAGE_TAG=develop tests/e2e/restore.sh
```
