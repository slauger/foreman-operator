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

## Not available yet

- End-to-end tests on Kind and k3s
- Qualification on OpenShift
- A PXE build test on a real L2 network
