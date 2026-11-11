CHART ?= charts/foreman-operator
IMAGE ?= ghcr.io/slauger/foreman-operator
VERSION ?= dev

CONTROLLER_GEN = go tool controller-gen
GOVULNCHECK = go tool govulncheck
SETUP_ENVTEST = go tool setup-envtest
ENVTEST_K8S_VERSION ?= 1.37.0
COVERAGE_THRESHOLD ?= 60
# GNU sed (on macOS: brew install gnu-sed)
SED ?= $(shell which gsed 2>/dev/null || echo sed)
PYTHON ?= python3
VENV ?= .venv

.PHONY: all
all: test

##@ Development

.PHONY: manifests
manifests: ## Generate CRD and RBAC manifests and copy them into the Helm chart.
	$(CONTROLLER_GEN) rbac:roleName=foreman-operator crd paths="./..." output:crd:dir=config/crd/bases output:rbac:dir=config/rbac
	# Drop the controller-gen version stamp (and the then empty annotations), otherwise every
	# controller-tools update changes all CRDs without any schema change.
	@$(SED) --version >/dev/null 2>&1 || { echo "GNU sed required (macOS: brew install gnu-sed)"; exit 1; }
	$(SED) -i -z -e 's/\n    controller-gen\.kubebuilder\.io\/version: [^\n]*//g' -e 's/\n  annotations:\n  \([^ ]\)/\n  \1/g' config/crd/bases/*.yaml
	cp config/crd/bases/*.yaml $(CHART)/crds/
	# The chart renders the generated rules, so RBAC cannot drift from the controllers.
	$(SED) -n '/^rules:/,$$p' config/rbac/role.yaml > $(CHART)/files/rules.yaml

.PHONY: generate
generate: ## Generate deepcopy methods.
	$(CONTROLLER_GEN) object paths="./api/..."

.PHONY: build
build: ## Build the operator binary.
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$(VERSION)" -o bin/manager ./cmd

.PHONY: run
run: manifests generate ## Run the operator against the current kubeconfig.
	go run ./cmd

.PHONY: fmt
fmt: ## Run go fmt.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet ## Run tests.
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test -race ./... -coverprofile cover.out

.PHONY: cover
cover: test ## Run tests and enforce the coverage threshold.
	./hack/check-coverage.sh cover.out $(COVERAGE_THRESHOLD)

##@ Quality

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: vulncheck
vulncheck: ## Run govulncheck.
	$(GOVULNCHECK) ./...

.PHONY: check-manifests
check-manifests: manifests generate ## Check for CRD, RBAC, and deepcopy drift.
	git diff --exit-code -- config api/v1alpha1/zz_generated.deepcopy.go $(CHART)/crds $(CHART)/files

.PHONY: helm-lint
helm-lint: ## Lint all Helm charts.
	@for chart in charts/*/; do helm lint "$$chart" || exit 1; done

.PHONY: helm-unittest
helm-unittest: ## Run Helm unit tests.
	helm unittest $(CHART)

.PHONY: ci
ci: lint vet cover check-manifests vulncheck helm-lint helm-unittest ## Run all CI checks locally.

##@ Images

.PHONY: image
image: ## Build the operator image for linux/amd64.
	docker build --platform linux/amd64 --build-arg VERSION=$(VERSION) -f images/foreman-operator/Containerfile -t $(IMAGE):$(VERSION) .

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
