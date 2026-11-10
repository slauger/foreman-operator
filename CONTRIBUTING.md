# Contributing to foreman-operator

This project is experimental. Expect breaking changes without notice.

## Getting Started

### Prerequisites

- Go 1.27+
- Helm
- Python 3 for the documentation
- GNU sed (on macOS: `brew install gnu-sed`)
- [golangci-lint](https://golangci-lint.run/)

### Clone and Build

```bash
git clone https://github.com/slauger/foreman-operator.git
cd foreman-operator
make test
```

### Checks

```bash
make test          # Unit tests
make lint          # Linting
make helm-lint     # Helm chart linting
make ci            # All CI checks
make docs-build    # Documentation in strict mode
make docs-serve    # Documentation with live reload
```

## Workflow

1. Branch from `develop` (`feat/<topic>` or `fix/<topic>`).
2. Use [Conventional Commits](https://www.conventionalcommits.org/).
3. Update the documentation in the same change.
4. Open a pull request against `develop`.

## Licensing

Contributions are licensed under Apache-2.0. Do not copy code, templates, or
text from GPL-3.0 projects such as `foremanctl` or `foreman-kubernetes`.
Reimplement behavior from its description instead.
