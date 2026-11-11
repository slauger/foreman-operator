# Agent Instructions

## Project status

This project is experimental. The `Foreman` and `SmartProxy` controllers exist as an MVP. The frontend is part of `Foreman`. Documentation must distinguish planned, implemented, and tested behavior, and name the environment a test ran in.

## Git Workflow

- **Development branch:** `develop`
- **Release branch:** `main`
- **Always branch from `origin/develop`**, never from `main`
- **PRs target `develop`**
- Branch naming: `feat/<topic>` or `fix/<topic>`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/).

```
feat: add SmartProxy controller
fix: use the CloudNativePG read-write service
docs: describe the provisioning pod layout
ci: add helm lint job
chore: update dependencies
refactor: simplify reconcile loop
test: add tests for the SmartProxy builder
```

- Lowercase subject, no trailing period
- No AI attribution trailers and no session links
- GPG-sign commits

## Pull Requests

- Title follows conventional commit format, under 72 characters
- Body has a `## Summary` with bullet points and a `## Test plan`
- No AI attribution footers

## Build & Test

```bash
make generate manifests   # regenerate deepcopy, CRD YAML, and RBAC; copy CRDs and rules into the chart
make build                # build bin/manager
make test                 # manifests, generate, fmt, vet, unit, fake-client, and envtest tests
make cover                # test, then enforce the coverage threshold (60 percent total)
make lint                 # golangci-lint
make helm-lint            # lint all charts
make helm-unittest        # helm-unittest for charts/foreman-operator
make ci                   # lint, vet, cover, check-manifests, vulncheck, helm-lint, helm-unittest
make run                  # run the operator against the current kubeconfig
make image                # build the operator image for linux/amd64
make docs-build           # build the documentation in strict mode
```

Always run `make generate manifests` after modifying `api/v1alpha1/*_types.go` or the `+kubebuilder:rbac` markers. The chart renders its RBAC rules from `charts/foreman-operator/files/rules.yaml`, which `make manifests` generates.

## End-to-end tests

The E2E test runs against the `e2e-openvox` k3s cluster with Multus, cert-manager, CloudNativePG, and Envoy Gateway. `setup.sh` refuses to run against any other cluster.

```bash
IMAGE_TAG=develop tests/e2e/setup.sh                  # prerequisites, CRDs, and the operator via Helm
OPERATOR=local IMAGE_TAG=develop tests/e2e/setup.sh   # same, without the in-cluster operator (use make run)
chainsaw test --config tests/e2e/chainsaw-config.yaml tests/e2e/mvp
```

`IMAGE_TAG` selects the operator and component images, for example `develop` or a feature branch tag.

## Project Structure

```
api/v1alpha1/              CRD type definitions (Go structs)
cmd/                       Operator entry point
internal/controller/       Reconcilers and builders for Foreman (including the frontend) and SmartProxy
internal/foremanapi/       Minimal Foreman API client with OAuth 1.0a
config/crd/bases/          Generated CRD YAML
config/rbac/               Generated RBAC role
config/samples/            Example resources
charts/foreman-operator/   Helm chart for the operator
charts/foreman-db-cnpg/    Helm chart for a CloudNativePG cluster
images/                    Containerfiles for the operator and component images
tests/e2e/                 Chainsaw E2E test and cluster setup
hack/                      Helper scripts and the PXE lab manifest
docs/                      Documentation (MkDocs Material)
```

## Licensing

This repository is Apache-2.0. Foreman, Smart Proxy, `foremanctl`, and `foreman-kubernetes` are GPL-3.0.

- Do not copy files or substantial parts from GPL-3.0 repositories.
- Reimplement behavior from its description.
- Foreman or Smart Proxy plugins belong in a separate GPL-3.0 repository.
- Images that ship GPL components must carry license texts and provide source.

## Text Quality

- **No Unicode em-dashes, smart quotes, or other non-ASCII punctuation.** Use plain ASCII. Emoji in `README.md` and the feature list are allowed.
- CI rejects zero-width characters, soft hyphens, and similar characters.

## Documentation

Every change that alters observable behavior updates the documentation in the same commit.

| Change | Update |
|---|---|
| CRD field added, removed, or renamed | the field table in `docs/reference/<kind>.md` |
| New condition or phase | the conditions table |
| New chart value | `values.yaml` comments, `values.schema.json`, and `charts/foreman-operator/README.md` |
| Feature added or changed | `docs/_snippets/features.md` and the copy in `README.md` |

When a documented claim cannot be verified against the code, say so in the text.
