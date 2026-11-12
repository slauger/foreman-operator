# Database

!!! note "Status"
    Implemented. The chart and the `Foreman` controller are used by the E2E test on a k3s cluster, where migration and web are verified.

PostgreSQL is provided by [CloudNativePG](https://cloudnative-pg.io/). A `Foreman` references a `Cluster` in its namespace.

```mermaid
flowchart LR
  Foreman -->|database.cluster.name| Cluster[CloudNativePG Cluster]
  Cluster --> RW["Service name-rw"]
  Cluster --> Secret["Secret name-app"]
```

## Create a cluster

```bash
helm install foreman-db charts/foreman-db-cnpg --namespace foreman --create-namespace
```

```yaml
spec:
  database:
    cluster:
      name: foreman-db
```

The operator reads the `uri` key of the `<name>-app` Secret, which CloudNativePG fills with a connection string for the `<name>-rw` Service, and passes it to Foreman with `sslmode=require`. `credentialsSecretRef` overrides the Secret name. Until the Secret exists, the `Foreman` stays in phase `Pending` with reason `WaitingForDatabase`.

## Decisions

- **PostgreSQL 16**, the major version used by `foremanctl`.
- **CloudNativePG only.** A second path for external servers can be added later if needed.
- **One cluster per application database.** Katello adds separate clusters for Candlepin and Pulp, which keeps backups and upgrades independent.

Required PostgreSQL extensions have not been audited yet. `postInitSQL` can add them.

## Backup

Use CloudNativePG continuous backups and `ScheduledBackup`. A restore also needs the secrets listed in [Upgrades](upgrades.md#backup-and-restore).

## Valkey

Foreman also needs Valkey for the Rails cache and for Dynflow, which makes it a required dependency. The operator consumes an existing instance through `spec.valkey` and does not deploy one. Like `foremanctl`, it uses database 4 for the cache and 6 for Dynflow. Authentication and TLS to Valkey are not supported yet.
