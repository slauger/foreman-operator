# Plugins

!!! note "Status"
    Target design. `Foreman.spec.plugins` exists in the API.

## Model

- `spec.plugins` lists the enabled plugins. Each plugin must be present in the image.
- The same plugin list applies to every workload, including recurring tasks. A task started with a different list fails.
- The operator keeps a catalog that maps a plugin to its Smart Proxy plugin and its dependencies.
- **Enabling a plugin is a migration.** It follows the same order as an upgrade, not a plain rollout.

## Extra plugins

Plugins are RPM-packaged gems with dependencies and assets, so mounting them from a volume does not fit. A plugin that is not in the default image requires a derived image, referenced by digest in `spec.image`.

Selecting plugins at runtime needs support in the image. It must be implemented in the images of this project.

## Candidates

| Plugin | Stage | Package status |
|---|---|---|
| `foreman_discovery` | Base | Resolved on UBI 9 |
| `foreman_remote_execution` | Base | Resolved on UBI 9 |
| `foreman_puppet` | To be decided | Resolved on UBI 9 |
| `foreman_bootdisk` | Later | Packages missing on UBI 9 |
| `katello` | [Katello](katello.md) | Not checked |

## Licensing

Plugins written for Foreman or Smart Proxy are likely derivative works of GPL-3.0 software. They belong in a separate GPL-3.0 repository.
