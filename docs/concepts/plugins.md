# Plugins

!!! note "Status"
    Implemented. The Foreman image contains a fixed plugin catalog, and `spec.plugins` selects which of them are loaded. The Smart Proxy image contains the Dynflow, Remote Execution SSH, and Ansible plugins for the `Execution` profile, see [Smart Proxy](smart-proxy.md#execution-profile).

## Model

- `spec.plugins` lists the enabled plugins. The API accepts only plugins from the catalog.
- The operator adds the dependencies of each plugin and sets the result as `FOREMAN_ENABLED_PLUGINS` on every Foreman workload and the migration Job.
- The bundler.d file of each packaged plugin loads it only if `FOREMAN_ENABLED_PLUGINS` lists it. The variable is always set, so an empty list keeps every installed plugin off. The image build fails if a plugin would load unconditionally.
- **Enabling a plugin is a migration.** The enabled plugins are part of the migration hash, so a change follows the same order as an upgrade, not a plain rollout. The migration Job also runs the seeds, which create the plugin's templates and permissions.
- Disabling a plugin leaves its tables and data in the database.

## Catalog

| Plugin | Also enables |
|---|---|
| `foreman_ansible` | `foreman_remote_execution`, `foreman-tasks` |
| `foreman_bootdisk` | |
| `foreman_discovery` | |
| `foreman_puppet` | |
| `foreman_remote_execution` | `foreman-tasks` |
| `foreman_templates` | |
| `foreman_webhooks` | |

Not in the catalog:

| Plugin | Reason |
|---|---|
| `foreman_openscap` | Needs `scap-security-guide`, which UBI 9 lacks |
| `katello` | See [Katello](katello.md) |

## Boot disks

`foreman_bootdisk` builds boot ISO images in the web pods. Its tools are in RHEL 9 but not in UBI 9: `syslinux` with `isohybrid`, `ipxe-bootimgs-x86`, `xorriso`, `dosfstools`, and `mtools` come from signed CentOS Stream 9 packages, with their other dependencies from UBI 9. The EFI boot image needs only the GRUB 2 and shim binaries, so `grubx64.efi`, `shimx64.efi`, and `mmx64.efi` from CentOS Stream 9 are copied to `/var/lib/foreman/bootdisk`, the default "Grub2 directory". The plugin itself is installed without the `grub2-efi-x64` dependency. The ISO is built in `/usr/share/foreman/tmp`, an `emptyDir`.

## Extra plugins

Plugins are RPM-packaged gems with dependencies and assets, so mounting them from a volume does not fit. A plugin outside the catalog requires a derived image and a new catalog entry in the API.

## Licensing

Plugins written for Foreman or Smart Proxy are likely derivative works of GPL-3.0 software. They belong in a separate GPL-3.0 repository.
