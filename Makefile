CONTROLLER_GEN = go tool controller-gen
GOVULNCHECK = go tool govulncheck
# GNU sed (on macOS: brew install gnu-sed)
SED ?= $(shell which gsed 2>/dev/null || echo sed)
PYTHON ?= python3
VENV ?= .venv

.PHONY: all
all: test

##@ Development

.PHONY: manifests
manifests: ## Generate CRD manifests.
	$(CONTROLLER_GEN) crd paths="./..." output:crd:dir=config/crd/bases
	# Drop the controller-gen version stamp (and the then empty annotations), otherwise every
	# controller-tools update changes all CRDs without any schema change.
	@$(SED) --version >/dev/null 2>&1 || { echo "GNU sed required (macOS: brew install gnu-sed)"; exit 1; }
	$(SED) -i -z -e 's/\n    controller-gen\.kubebuilder\.io\/version: [^\n]*//g' -e 's/\n  annotations:\n  \([^ ]\)/\n  \1/g' config/crd/bases/*.yaml

.PHONY: generate
generate: ## Generate deepcopy methods.
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	go test ./... -coverprofile cover.out

##@ Quality

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: vulncheck
vulncheck: ## Run govulncheck.
	$(GOVULNCHECK) ./...

.PHONY: check-manifests
check-manifests: manifests generate ## Check for CRD and deepcopy drift.
	git diff --exit-code -- config/crd/bases api/v1alpha1/zz_generated.deepcopy.go

.PHONY: helm-lint
helm-lint: ## Lint all Helm charts.
	@for chart in charts/*/; do helm lint "$$chart" || exit 1; done

.PHONY: ci
ci: lint vet test check-manifests vulncheck helm-lint ## Run all CI checks locally.

##@ Documentation

$(VENV)/bin/mkdocs:
	$(PYTHON) -m venv $(VENV)
	$(VENV)/bin/pip install --quiet mkdocs-material

.PHONY: docs-serve
docs-serve: $(VENV)/bin/mkdocs ## Serve the documentation locally.
	$(VENV)/bin/mkdocs serve

.PHONY: docs-build
docs-build: $(VENV)/bin/mkdocs ## Build the documentation in strict mode.
	$(VENV)/bin/mkdocs build --strict

##@ Help

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
