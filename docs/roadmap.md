# Roadmap

The focus of the first stages is Foreman with Smart Proxy. Versions are intended goals and may change.

| Version | Content | Done when | State |
|---|---|---|---|
| v0.0 Docs | Repository, documentation, CRD types | This page is online | Done |
| v0.0 Spike | Manual manifests on k3s, `frontend` image, dnsmasq next to an unprivileged proxy, UBI 9 Foreman and proxy images | A test client gets a lease and a boot file over Multus | Done on a single-node bridge, see [Testing](development/testing.md#pxe-lab) |
| v0.1 | Operator with `Foreman` and `Frontend`, Dynflow, cert-manager, CloudNativePG, migration before rollout | Foreman runs, login and API work | Implemented as MVP, verified on k3s through a Gateway API TLSRoute |
| v0.2 | `SmartProxy` profile `Provisioning`, registration, DHCP, TFTP, DNS | A complete PXE build on k3s, driven only by custom resources | Implemented as MVP, E2E test passes on k3s with a fake VLAN |
| v0.3 | OpenShift qualification, dedicated SCC for the proxy | The same PXE build on OpenShift | Open |
| v0.4 | Hosts queue, profile `Execution`, plugin catalog, recurring tasks | Remote Execution works, plugin activation runs through a migration | Open |
| v0.5 | `Release`, backup, and restore | Upgrade and restore drills pass | Open |
| v0.6 | Migration from existing installations | Rehearsal with a production-sized copy | Open |
| v0.7+ | Katello with `Candlepin`, `Pulp`, content proxies | Sync and host registration work | Open |

The MVP is v0.1 and v0.2: `Foreman`, `Frontend`, and `SmartProxy` with DHCP, TFTP, and DNS, created only from custom resources. DHCP and TFTP can be tested only to a limited extent on the public E2E cluster, which uses a node-local bridge as a fake VLAN. A real PXE build is planned on a dedicated VLAN in a homelab.

## Principles

- **Spike before operator.** Whether DHCP and TFTP work in a pod on Multus was the largest unknown. The spike answered it before the API was fixed.
- **Check OpenShift behavior early.** The operator never sets a UID for Foreman, Frontend, or proxy containers, so they run under the UID the platform assigns.

## Open decisions

- Whether `foreman_discovery` moves into a later version
- BMC: `ipmitool` and `freeipmi` are not in UBI 9, and the Smart Proxy image is installed without `rubygem-rubyipmi`. The BMC feature can be declared but is not usable.
- Whether the operator should stay separate from `foreman-kubernetes`
