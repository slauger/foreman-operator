# Testing

## Checks

| Check | Command |
|---|---|
| Unit tests, fake-client controller tests, and CEL validation with envtest | `make test` |
| Tests with the coverage gate of 60 percent total | `make cover` |
| Linting | `make lint` |
| Generated files | `make check-manifests` |
| Vulnerabilities | `make vulncheck` |
| Helm charts | `make helm-lint` |
| Helm unit tests | `make helm-unittest` |
| Documentation | `make docs-build` |
| All of the above except documentation | `make ci` |

The controller tests run the reconcilers against a fake client and cover about 86 percent of `internal/controller`. CEL rules of the CRDs are tested against a real API server with envtest. The Foreman API client is tested against an HTTP test server, including the OAuth signature.

## End-to-end test

The E2E test is a [Chainsaw](https://kyverno.github.io/chainsaw/) test in `tests/e2e/mvp`. It runs against the `e2e-openvox` cluster: a single k3s node with Multus, cert-manager, CloudNativePG, and Envoy Gateway. `tests/e2e/setup.sh` refuses to run against any other cluster.

```bash
IMAGE_TAG=develop tests/e2e/setup.sh
chainsaw test --config tests/e2e/chainsaw-config.yaml tests/e2e/mvp
```

`setup.sh` creates the namespace `foreman-e2e` with Pod Security level `privileged`, a TSIG key, the CloudNativePG cluster from `charts/foreman-db-cnpg`, the prerequisites in `tests/e2e/setup/` (internal CA, Valkey, a fake provisioning network, a BusyBox client, and BIND), and the Gateway. It applies the CRDs and installs the operator with Helm, using `IMAGE_TAG` for the operator and all component images. With `OPERATOR=local`, it skips the Helm install, so the operator can run locally with `make run`.

| Step | Checks | Verified on the cluster |
|---|---|---|
| Foreman migrates and runs | Phase `Running` after the migration Job, for the current generation | Passed |
| Puma has no TCP listener | The web pod listens only on ports 8080 and 8443 | Passed |
| The frontend container serves Foreman over the Gateway | `/api/v2/ping` through the TLSRoute | Passed |
| Recurring tasks run as CronJobs | The four CronJobs exist, and a Job created from `cron-hourly` completes | Passed |
| Only the enabled plugins load | The plugin list of `/api/v2/plugins` matches `spec.plugins` with dependencies | Passed |
| The compute resource providers are available | KubeVirt, Proxmox, and VMware | Passed |
| Bootdisk builds a generic boot ISO | `/api/bootdisk/generic` returns an ISO 9660 image | Passed |
| Smart Proxy is registered with its features | Phase `Running`, `foremanID` set, features `BMC DHCP DNS Discovery Logs TFTP Templates` in Foreman | Passed |
| DHCP reservation reaches a client on the fake VLAN | A reservation created through the proxy API gives the client its address | Passed |
| TFTP serves the boot loader | The client downloads `pxelinux.0` | Passed |
| A discovered host reaches Foreman through the proxy | The client posts facts to the proxy, and the discovered host appears in Foreman | Passed |
| DNS record lands in BIND | A record created through the proxy API resolves | Passed |
| DHCP state survives a new pod | After a rollout restart, the lease file and the reservation are still on the volume | Passed |
| The BMC provider finds ipmitool | `ipmitool -V` runs in the proxy container | Passed |
| The proxy probes complete the TLS handshake | No SSL errors in the proxy log | Passed |
| The Execution proxy is registered with its features | Features `Ansible Dynflow Logs Script` in Foreman | Passed |
| A Remote Execution job runs on the target over SSH | `Run Command - Script Default` succeeds on an Alpine target over SSH | Passed |
| An Ansible job runs on the target | `Run Command - Ansible Default` succeeds on the same target | Passed |
| Deleting the Smart Proxy removes it from Foreman | The finalizer unregisters the proxy | Passed |

## Upgrade test

`tests/e2e/upgrade.sh` deletes the namespace `foreman-e2e`, installs the released chart `FROM_VERSION` (default `0.1.0`) from the registry with `setup.sh`, creates a Foreman with Remote Execution and an Execution proxy from `tests/e2e/upgrade/resources.yaml`, and creates a domain through the API. It then upgrades the operator to `IMAGE_TAG` with `setup.sh` and waits until the migration completed and `Verified` is `True`. It checks `status.foremanVersion`, that the domain still exists, and that the proxy refreshed its features.

```bash
IMAGE_TAG=develop tests/e2e/upgrade.sh
IMAGE_TAG=develop tests/e2e/setup.sh
chainsaw test --config tests/e2e/chainsaw-config.yaml tests/e2e/mvp
```

Run `setup.sh` again before the MVP test, since the upgrade test leaves its own resources in the namespace.

## Restore drill

`tests/e2e/restore.sh` runs the restore order of [Backup and Restore](../guides/backup-restore.md#restore-drill) in an empty namespace with `IMAGE_TAG`. Like the upgrade test, it leaves its resources in the namespace.

The E2E, upgrade, and restore tests run by hand before a pull request is merged. CI runs only the checks above.

## PXE lab

`hack/pxe-lab.yaml` builds a fake provisioning VLAN on a single node: a Linux bridge `pxe0` without uplink, attached through Multus. It runs dnsmasq on UBI 9 next to an unprivileged stand-in for the Smart Proxy, and a BusyBox client. It predates the operator and uses manual manifests.

Verified on k3s with Multus thick v4.3.0:

- The client gets a DHCP lease with boot file and TFTP server, and downloads the boot file via TFTP.
- dnsmasq runs as user `dnsmasq` and keeps only `NET_ADMIN` and `NET_RAW`.
- Reservations written in the layout of `smart_proxy_dhcp_dnsmasq` take effect when added, changed, and deleted.
- The proxy's `reload_cmd` only creates a trigger file. A supervisor in the dnsmasq container turns it into a SIGHUP, so the proxy needs neither a shared process namespace nor `CAP_KILL`.

The dnsmasq container needs the PSA level `privileged`, because `NET_ADMIN` is not allowed below it. This was not tested with a real VLAN, a real NIC, or real firmware.

## Not available yet

- E2E tests in CI, for example on Kind
- Qualification on OpenShift
- A PXE build test with a real host on a real L2 network
