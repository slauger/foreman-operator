# foreman-db-cnpg

Creates a [CloudNativePG](https://cloudnative-pg.io/) `Cluster` for Foreman. The
CloudNativePG operator must be installed in the cluster.

```bash
helm install foreman-db charts/foreman-db-cnpg --namespace foreman --create-namespace
```

| Value | Default | Description |
|---|---|---|
| `name` | release name | Cluster resource name |
| `imageName` | `ghcr.io/cloudnative-pg/postgresql:16` | PostgreSQL image |
| `instances` | `1` | Number of PostgreSQL instances |
| `storage.size` | `10Gi` | PVC size for each instance |
| `storage.storageClass` | `""` | Storage class, empty uses the cluster default |
| `database` | `foreman` | Database name |
| `postInitSQL` | `[]` | Extra SQL statements to run after database creation |
