# Local Setup

## Prerequisites

- Go 1.27+
- Helm with the [helm-unittest](https://github.com/helm-unittest/helm-unittest) plugin
- Python 3
- GNU sed (on macOS: `brew install gnu-sed`)
- [golangci-lint](https://golangci-lint.run/)
- Docker, for `make image`
- [Chainsaw](https://kyverno.github.io/chainsaw/) and `envsubst`, for the E2E test

`controller-gen`, `setup-envtest`, and `govulncheck` run through `go tool` and need no separate installation.

## Common tasks

```bash
make generate manifests   # regenerate deepcopy, CRD YAML, and RBAC, copy CRDs and rules into the chart
make test                 # generate, format, vet, and test
make cover                # test with the coverage gate
make run                  # run the operator against the current kubeconfig
make build                # build bin/manager
make image                # build the operator image for linux/amd64
make docs-serve           # documentation with live reload
make help                 # list all targets
```

`make docs-serve` and `make docs-build` create a virtual environment in `.venv`.

`make run` uses the current kubeconfig and the default image tag `develop` for the component images. The cluster needs the CRDs from `charts/foreman-operator/crds/`.

## Repository layout

```
api/v1alpha1/              CRD type definitions
cmd/                       Operator entry point
internal/controller/       Reconcilers and builders
internal/foremanapi/       Foreman API client with OAuth 1.0a
config/crd/bases/          Generated CRD YAML
config/rbac/               Generated RBAC role
config/samples/            Example resources
charts/foreman-operator/   Helm chart for the operator
charts/foreman-db-cnpg/    Helm chart for a CloudNativePG cluster
images/                    Containerfiles
tests/e2e/                 Chainsaw E2E test and cluster setup
hack/                      Helper scripts and the PXE lab manifest
docs/                      Documentation
```
