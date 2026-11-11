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
| Foreman migrates and runs | Phase `Running` after the migration Job | Yes, with the earlier separate `Frontend` |
| Puma has no TCP listener | The web pod listens only on ports 8080 and 8443 | Covered by the test, result not recorded yet |
| The frontend container serves Foreman over the Gateway | `/api/v2/ping` through the TLSRoute | Yes with the earlier separate `Frontend`; not recorded yet for the sidecar |
| Smart Proxy is registered with its features | Phase `Running`, `foremanID` set, features `DHCP DNS Logs TFTP Templates` in Foreman | Passed |
| DHCP reservation reaches a client on the fake VLAN | A reservation created through the proxy API gives the client its address | Passed |
| TFTP serves the boot loader | The client downloads `pxelinux.0` | Passed |
| DNS record lands in BIND | A record created through the proxy API resolves | Passed |
| Deleting the Smart Proxy removes it from Foreman | The finalizer unregisters the proxy | Passed |

Independent of the Chainsaw steps, the OAuth signature of the operator was verified against the real Foreman on the cluster: correctly signed requests are accepted and wrongly signed requests are rejected. The full test passed on e2e-openvox both against an existing installation and from scratch, with a new namespace and an empty database, before the frontend moved into the web pods. A run with the sidecar is not recorded yet.

## PXE lab

`hack/pxe-lab.yaml` builds a fake provisioning VLAN on a single node: a Linux bridge `pxe0` without uplink, attached through Multus. It runs dnsmasq on UBI 9 next to an unprivileged stand-in for the Smart Proxy, and a BusyBox client. It predates the operator and uses manual manifests.

Verified on k3s with Multus thick v4.3.0:

- The client gets a DHCP lease with boot file and TFTP server, and downloads the boot file via TFTP.
- dnsmasq runs as user `dnsmasq` and keeps only `NET_ADMIN` and `NET_RAW`.
- Reservations written in the layout of `smart_proxy_dhcp_dnsmasq` take effect when added, changed, and deleted.
- The proxy's `reload_cmd` only creates a trigger file. A supervisor in the dnsmasq container turns it into a SIGHUP, so the proxy needs neither a shared process namespace nor `CAP_KILL`.

The dnsmasq container needs the PSA level `privileged`, because `NET_ADMIN` is not allowed below it. This was not tested with a real VLAN, a real NIC, or real firmware.

## Not available yet

- Running the Smart Proxy steps of the E2E test on the cluster
- E2E tests on Kind
- Qualification on OpenShift
- A PXE build test with a real host on a real L2 network
