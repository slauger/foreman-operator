# Local Setup

## Prerequisites

- Go 1.27+
- Helm
- Python 3
- GNU sed (on macOS: `brew install gnu-sed`)
- [golangci-lint](https://golangci-lint.run/)

## Common tasks

```bash
make generate manifests   # regenerate deepcopy and CRD YAML
make test                 # generate, format, vet, and test
make docs-serve           # documentation with live reload
```

`make docs-serve` and `make docs-build` create a virtual environment in `.venv`.

## Repository layout

```
api/v1alpha1/              CRD type definitions
config/crd/bases/          Generated CRD YAML
config/samples/            Example resources
charts/foreman-db-cnpg/    Helm chart for a CloudNativePG cluster
docs/                      Documentation
```

`cmd/`, `internal/controller/`, `images/`, and `tests/e2e/` are added with the first code that needs them.
