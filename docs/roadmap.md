# Roadmap

The focus is Foreman with Smart Proxies for provisioning and Remote Execution. Katello follows later. Stages are intended goals and may change. Open work is tracked in [GitHub issues](https://github.com/slauger/foreman-operator/issues).

## Done

| Stage | Content | State |
|---|---|---|
| Spike | Manual manifests on k3s: dnsmasq next to an unprivileged proxy on Multus, UBI 9 images | A test client got a lease and a boot file, see [Testing](development/testing.md#pxe-lab) |
| Foreman | `Foreman` with the frontend sidecar, Dynflow, cert-manager, CloudNativePG, migration before rollout | Released in `0.1.0` |
| Provisioning | `SmartProxy` profile `Provisioning`: registration, DHCP and TFTP through dnsmasq, DNS through nsupdate, BMC | Released in `0.1.0`; E2E on a fake VLAN |
| Execution | Profile `Execution` with Remote Execution and Ansible over SSH, hosts queue | Released in `0.1.0`; E2E runs both job types |
| Plugin catalog | Activation through a migration; Remote Execution, Ansible, Templates, Webhooks, and Puppet | Released in `0.1.0` |
| More plugins | Boot Disk with its tools, Discovery with the proxy feature, KubeVirt, Proxmox, VMware | Released in `0.2.0` |
| Recurring tasks | CronJobs for `cron:hourly` to `cron:monthly` | Released in `0.2.0` |
| Upgrades | Preflight, fence, migration, roll forward guard, smoke checks; upgrade E2E from `0.1.0` | Released in `0.2.0` |
| Restore | Restore order documented and drilled with a logical dump | Released in `0.2.0` |
| Releases | semantic-release, signed images and charts in GHCR, SBOMs, source references | Done |
| Version tracking | Pinned Foreman and plugin RPMs, tracked by Renovate | Released in `0.2.0` |

## Towards a beta

A beta means: a real host is built, the operator runs on OpenShift, and an operator can run it without reading the code. These items are needed for that, roughly in this order.

| Item | Why | Issue |
|---|---|---|
| Sign `develop` images | The repository is public, so the reason for `sign: false` is gone | [#50](https://github.com/slauger/foreman-operator/issues/50) |
| Scheduling | `nodeSelector`, tolerations, and affinity. A provisioning proxy must run on a node that has the VLAN trunked | [#52](https://github.com/slauger/foreman-operator/issues/52) |
| A complete PXE build | No host has been built end to end. A KubeVirt VM on the E2E cluster, or a real VLAN in a homelab | [#38](https://github.com/slauger/foreman-operator/issues/38) |
| OpenShift qualification | A dedicated SCC for dnsmasq, an insecure Route for `/unattended`, the E2E test on OpenShift | [#12](https://github.com/slauger/foreman-operator/issues/12) |
| E2E in CI | The E2E tests run by hand on one shared cluster. A Kind job for Foreman and the `Execution` proxy, without Multus, would catch regressions on every pull request | [#51](https://github.com/slauger/foreman-operator/issues/51) |
| Foreman 5.1 | The first minor upgrade through a Renovate pull request and the upgrade flow, once Foreman 5.1 is released | [#60](https://github.com/slauger/foreman-operator/issues/60) |

## Operations

Part of the [original concept](#comparison-with-the-concept) or found missing while testing, not implemented yet. Each can be added without changing the API in an incompatible way.

| Item | Purpose | Issue |
|---|---|---|
| PodDisruptionBudget for web | Keeps one web pod during node drains | [#53](https://github.com/slauger/foreman-operator/issues/53) |
| Default NetworkPolicy | Limits Foreman and the proxies to the expected peers. Puma no longer needs it, since it has no TCP listener | [#54](https://github.com/slauger/foreman-operator/issues/54) |
| Metrics | Migration phase and duration, upgrade blocks, registration errors; a ServiceMonitor in the chart | [#55](https://github.com/slauger/foreman-operator/issues/55) |
| Valkey authentication and TLS | Needed for shared or managed Valkey instances | [#56](https://github.com/slauger/foreman-operator/issues/56) |
| Backups in the database chart | `ScheduledBackup` with the Barman Cloud plugin as optional values of `foreman-db-cnpg` | [#57](https://github.com/slauger/foreman-operator/issues/57) |
| Off-cluster Smart Proxies | Export a certificate bundle for a proxy outside the cluster, like the `auth-bundle` command of `foremanctl` | [#58](https://github.com/slauger/foreman-operator/issues/58) |
| `Release` | A kind for compatible image sets of Foreman, frontend, and Smart Proxy, and the proxy upgrade after Foreman | [#59](https://github.com/slauger/foreman-operator/issues/59) |

## Later

| Item | Notes |
|---|---|
| Migration from existing installations | Plan in [Migration](concepts/migration.md); rehearsal against a production-sized copy |
| Katello | `Candlepin`, `Pulp`, `/pub` ([#33](https://github.com/slauger/foreman-operator/issues/33)), and content proxies, see [Katello](concepts/katello.md) |
| More Smart Proxy providers | Infoblox for existing DHCP, `dns_nsupdate_gss` with a keytab, Remote Execution pull mode over MQTT |
| External authentication | Kerberos or SAML in the frontend |
| More plugins | `foreman_openscap` once `scap-security-guide` is available |

## Comparison with the concept

The original design proposal for this operator, written as input for the [foreman-kubernetes RFC](https://community.theforeman.org/t/rfc-first-class-kubernetes-deployment-for-foreman-katello-candlepin-and-pulp/47739), differs from the implementation in these points:

| Concept | Implementation | Reason |
|---|---|---|
| Separate `Frontend` kind with its own replicas | Apache as a sidecar in the web pods | Puma has no TCP listener, so no pod can forge the client certificate headers ([#7](https://github.com/slauger/foreman-operator/issues/7)) |
| Dynflow only when a plugin needs it | Orchestrator and one worker always run | Foreman core uses Dynflow as its ActiveJob backend |
| ISC DHCP with OMAPI | dnsmasq with `smart_proxy_dhcp_dnsmasq` | ISC DHCP is end of life and not in UBI 9; one subnet per proxy as a consequence |
| PostgreSQL from CloudNativePG or external | CloudNativePG only | One path to test. An external path can be added later |
| Platform detection with a manual override | Detection only | No case needed an override yet |
| Smoker tests on Kind, k3s, and OpenShift | Chainsaw tests on one k3s cluster | Smoker targets a Katello installation. Kind and OpenShift are open, see above |
| HPA, PDB, and NetworkPolicy from `Foreman` | Not implemented | See [Operations](#operations) |
| Upstream images | Own UBI 9 images | Random UID support and the provisioning tools were missing upstream |

## Principles

- **Spike before operator.** Whether DHCP and TFTP work in a pod on Multus was the largest unknown. The spike answered it before the API was fixed.
- **Check OpenShift behavior early.** The operator never sets a UID for Foreman, frontend, or proxy containers, so they run under the UID the platform assigns.
- **Test on a cluster.** A feature counts as done when an E2E step passes on the test cluster, not when the controller tests pass.

## Open decisions

- Whether the operator should stay separate from `foreman-kubernetes`, or be proposed there
- Whether KubeVirt may be installed on the shared E2E cluster for [#38](https://github.com/slauger/foreman-operator/issues/38)
- Whether `Release` is still needed once the chart pins all images to one version ([#59](https://github.com/slauger/foreman-operator/issues/59))
