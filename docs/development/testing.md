# Testing

## Available today

| Check | Command |
|---|---|
| Unit tests and CEL validation with envtest | `make test` |
| Linting | `make lint` |
| Generated files | `make check-manifests` |
| Vulnerabilities | `make vulncheck` |
| Helm charts | `make helm-lint` |
| Documentation | `make docs-build` |
| All of the above except documentation | `make ci` |

## PXE lab

`hack/spike/pxe-lab.yaml` builds a fake provisioning VLAN on a single node: a Linux bridge `pxe0` without uplink, attached through Multus. It runs dnsmasq on UBI 9 next to an unprivileged stand-in for the Smart Proxy, and a busybox client.

Verified on k3s with Multus thick v4.3.0:

- The client gets a DHCP lease with boot file and TFTP server, and downloads the boot file via TFTP.
- dnsmasq runs as user `dnsmasq` and keeps only `NET_ADMIN` and `NET_RAW`.
- Reservations written in the layout of `smart_proxy_dhcp_dnsmasq` take effect when added, changed, and deleted.
- The proxy's `reload_cmd` only creates a trigger file. A supervisor in the dnsmasq container turns it into a SIGHUP, so the proxy needs neither a shared process namespace nor `CAP_KILL`.

The dnsmasq container needs the PSA level `privileged`, because `NET_ADMIN` is not allowed below it. This was not tested with a real VLAN, a real NIC, or real firmware.

## Not available yet

- End-to-end tests on Kind and k3s
- Qualification on OpenShift
- A PXE build test on a real L2 network
