# Roadmap

The focus of the first stages is Foreman with Smart Proxy. Versions are intended goals and may change.

| Version | Content | Done when |
|---|---|---|
| v0.0 Docs | Repository, documentation, CRD types | This page is online |
| v0.0 Spike | Manual manifests on k3s, `frontend` and `dnsmasq` images, UBI 9 Foreman and proxy images | A test host boots through PXE over Multus |
| v0.1 | Operator with `Foreman` and `Frontend`, Dynflow, cert-manager, CloudNativePG, migration before rollout | Foreman runs, login and API work |
| v0.2 | `SmartProxy` profile `Provisioning`, registration, DHCP, TFTP, DNS, BMC | A complete PXE build on k3s, driven only by custom resources |
| v0.3 | OpenShift qualification, dedicated SCC for the proxy | The same PXE build on OpenShift |
| v0.4 | Hosts queue, profile `Execution`, plugin catalog | Remote Execution works, plugin activation runs through a migration |
| v0.5 | `Release`, backup, and restore | Upgrade and restore drills pass |
| v0.6 | Migration from existing installations | Rehearsal with a production-sized copy |
| v0.7+ | Katello with `Candlepin`, `Pulp`, content proxies | Sync and host registration work |

The MVP is v0.1 and v0.2: `Foreman`, `Frontend`, and `SmartProxy` with DHCP and TFTP, created only from custom resources. DHCP and TFTP can be tested only to a limited extent on the public e2e cluster. A real PXE build is planned on a dedicated VLAN in a homelab.

## Principles

- **Spike before operator.** Whether DHCP and TFTP work in a pod on Multus is the largest unknown. It is answered before the API is fixed.
- **Check OpenShift behavior early.** CI runs under an arbitrary UID from v0.1 on.

## Open decisions

- Whether `foreman_discovery` moves into v0.2
- BMC without `ipmitool` or `freeipmi` in UBI 9
- Whether the operator should stay separate from `foreman-kubernetes`
