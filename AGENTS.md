# Agent Instructions

## Project status

This project is experimental. No controllers exist yet. Documentation must
distinguish planned, implemented, and tested behavior.

## Git Workflow

- **Development branch:** `develop`
- **Release branch:** `main`
- **Always branch from `origin/develop`**, never from `main`
- **PRs target `develop`**
- Branch naming: `feat/<topic>` or `fix/<topic>`

## Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/).

```
feat: add Frontend controller
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
make generate manifests   # regenerate deepcopy and CRD YAML
go build ./...
go vet ./...
go test ./...
make ci                   # lint, vet, test, check-manifests, vulncheck, helm-lint
make docs-build           # build the documentation in strict mode
```

Always run `make generate manifests` after modifying `api/v1alpha1/*_types.go`.

## Project Structure

```
api/v1alpha1/              CRD type definitions (Go structs)
config/crd/bases/          Generated CRD YAML
config/samples/            Example resources
charts/foreman-db-cnpg/    Helm chart for a CloudNativePG cluster
docs/                      Documentation (MkDocs Material)
```

`cmd/`, `internal/controller/`, `images/`, and `tests/e2e/` are added with the
first code that needs them.

## Licensing

This repository is Apache-2.0. Foreman, Smart Proxy, `foremanctl`, and
`foreman-kubernetes` are GPL-3.0.

- Do not copy files or substantial parts from GPL-3.0 repositories.
- Reimplement behavior from its description.
- Foreman or Smart Proxy plugins belong in a separate GPL-3.0 repository.
- Images that ship GPL components must carry license texts and provide source.

## Text Quality

- **No Unicode em-dashes, smart quotes, or other non-ASCII punctuation.** Use
  plain ASCII. Emoji in `README.md` and the feature list are allowed.
- CI rejects zero-width characters, soft hyphens, and similar characters.

## Documentation

Every change that alters observable behavior updates the documentation in the
same commit.

| Change | Update |
|---|---|
| CRD field added, removed, or renamed | the field table in `docs/reference/<kind>.md` |
| New condition or phase | the conditions table |
| New chart value | `values.yaml` comments and `values.schema.json` |
| Feature added or changed | `docs/_snippets/features.md` and the copy in `README.md` |

When a documented claim cannot be verified against the code, say so in the text.
