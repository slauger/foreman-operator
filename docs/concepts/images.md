# Container Images

!!! note "Status"
    No images exist yet. The package results below come from repository metadata on 2026-11-10. They compare package names without versions and are not a real installation.

The upstream Foreman images are based on CentOS Stream 10, declare named users, and have no permissions for random UIDs. This project builds its own images on UBI 9.

| Image | Base | Content |
|---|---|---|
| `foreman-operator` | UBI 9 minimal | Operator binary |
| `foreman` | UBI 9 | Foreman RPMs, selected plugins |
| `foreman-proxy` | UBI 9 | Smart Proxy RPMs, selected plugins |
| `frontend` | UBI 9 | `httpd`, `mod_ssl`, own configuration |
| `dnsmasq` | UBI 9 | `dnsmasq` for DHCP and TFTP |
| `boot-files` | CentOS Stream 9 | Static bootloader files only |

## Package availability

Checked against UBI 9 BaseOS, AppStream, and CodeReady Builder, plus the Foreman 5.0 EL9 repositories.

| Package set | Result |
|---|---|
| `foreman`, `foreman-service`, `foreman-postgresql`, `foreman-redis`, `foreman-dynflow-sidekiq` | Resolved |
| `foreman_discovery`, `foreman_puppet`, `foreman_remote_execution` | Resolved |
| `foreman-proxy` | Open: `rubygem-ruby-libvirt` needs libvirt, `rubygem-rubyipmi` needs `ipmitool`. Only part of the list was reviewed |
| `foreman-bootloaders-redhat` | Open: GRUB and shim packages are missing |
| `foreman_bootdisk` | Open: five packages are missing |
| `dnsmasq`, `httpd`, `mod_ssl` | In UBI 9 |
| `dhcp-server`, `tftp-server`, `kea`, `freeipmi`, `ipmitool` | Not in UBI 9 |

UBI 10 has not been checked.

## Random UID support

Images follow the usual pattern: a numeric `USER <uid>:0`, group 0 owning writable paths with `chmod -R g=u`, and `HOME` set explicitly.

## Licensing

The images contain GPL-licensed software, including Foreman and Smart Proxy. Containerfiles in this repository are Apache-2.0. Each published image that ships GPL components must carry the license texts and make the corresponding source available, for example as a source image built from the SRPMs. RPMs are installed unpatched.
