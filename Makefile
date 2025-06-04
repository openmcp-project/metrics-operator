#
# =========================
# Metrics Operator Makefile
# =========================
#
# This Makefile provides targets for building, testing, deploying, and developing the metrics-operator.
# It is designed for contributors and maintainers working with a Kubebuilder-based project.
#
# Key Target Categories:
#   - General: Help, all, and core build/test targets
#   - Build: Build binaries and Docker images
#   - Development: Local dev cluster, install, and test helpers
#   - Deployment: Install/uninstall/deploy/undeploy to Kubernetes
#   - Linting: Code quality and formatting
#   - Helm: Helm chart packaging and install
#   - Crossplane: Crossplane installation and provider setup
#
# Usage:
#   make help         # Show all available targets and their descriptions
#   make build        # Build the operator binary
#   make test         # Run all tests
#   make deploy       # Deploy the operator to your current K8s context
#   make dev-local    # Setup a local dev cluster and install CRDs
#   make lint         # Run all linters
#   make helm-install-local # Install Helm chart locally
#
# For more details, see the README.md.

PROJECT_NAME := metrics
PROJECT_FULL_NAME := metrics-operator

# Image URL to use all building/pushing image targets
IMG_VERSION ?= dev
IMG_BASE ?= $(PROJECT_FULL_NAME)
IMG ?= $(IMG_BASE):$(IMG_VERSION)
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.27.1

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

# Tool Binaries and Versions
# --------------------------
# These variables define the locations and versions of required CLI tools.
# You can override them via environment variables if needed.
#
# KUBECTL:        Path to kubectl (default: kubectl in PATH)
# KIND:           Path to kind (default: kind in PATH)
# KUSTOMIZE:      Path to kustomize (default: ./bin/kustomize)
# CONTROLLER_GEN: Path to controller-gen (default: ./bin/controller-gen)
# ENVTEST:        Path to setup-envtest (default: ./bin/setup-envtest)
# GOTESTSUM:      Path to gotestsum (default: ./bin/gotestsum)
# GOLANGCILINT:   Path to golangci-lint (default: ./bin/golangci-lint)
#
# To update a tool version, change the corresponding *_VERSION variable below.
#
KUBECTL ?= kubectl
KIND ?= kind
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOTESTSUM ?= $(LOCALBIN)/gotestsum
GOLANGCILINT ?= $(LOCALBIN)/golangci-lint

KUSTOMIZE_VERSION ?= v5.4.1
CONTROLLER_TOOLS_VERSION ?= v0.17.2
GOLANGCILINT_VERSION ?= v2.0.2

.PHONY: all
all: build ## Build the operator binary (default target)

##@ General

.PHONY: help
help: ## Display this help message and list all available targets.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build the operator binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-docker-binary
build-docker-binary: manifests generate fmt vet ## Build the Linux/amd64 manager binary for Docker image.
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager-linux.amd64 cmd/main.go

.PHONY: docker-build
# Build the Docker image for the operator. Use IMG to override the image name.
docker-build: build-docker-binary test ## Build Docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
# Push the Docker image to a registry. Use IMG to override the image name.
docker-push: ## Push Docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

.PHONY: docker-buildx
# Build and tag Docker images for multiple platforms using Docker Buildx.
docker-buildx: ## Build and tag Docker image for each platform locally using --load
	sed '1 s/^FROM/FROM --platform=$${BUILDPLATFORM}/' Dockerfile > Dockerfile.cross
	$(CONTAINER_TOOL) buildx create --name project-v3-builder || true
	$(CONTAINER_TOOL) buildx use project-v3-builder
	@for platform in $(PLATFORMS); do \
		tag="$(IMG)-$$(echo $$platform | tr / -)"; \
		echo "Building $$tag for $$platform"; \
		$(CONTAINER_TOOL) buildx build --platform=$$platform --tag $$tag --load -f Dockerfile.cross .; \
	done
	$(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) crd paths="./..." output:crd:artifacts:config=cmd/embedded/crds

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run all Go tests with coverage.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

.PHONY: dev-local
dev-local: ## Create a local dev cluster and install CRDs.
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(MAKE) install

.PHONY: dev-local-all
dev-local-all: ## Full local dev environment: cluster, CRDs, Crossplane, providers, and sample resources.
	$(MAKE) dev-clean
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(MAKE) install
	$(MAKE) crossplane-install
	$(MAKE) crossplane-provider-install
	$(MAKE) crossplane-provider-sample
	$(MAKE) dev-namespace
	$(MAKE) dev-secret
	$(MAKE) dev-operator-namespace
	$(MAKE) dev-basic-metric
	$(MAKE) dev-managed-metric

.PHONY: dev-clean
dev-clean: ## Delete the local dev cluster.
	$(KIND) delete cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-run
dev-run: ## Run the operator locally (for debugging).
	go run ./cmd/main.go

.PHONY: dev-secret
dev-secret: ## Apply the example secret for local dev.
	kubectl apply -f examples/secret.yaml

.PHONY: dev-namespace
dev-namespace: ## Apply the example namespace for local dev.
	kubectl apply -f examples/namespace.yaml

.PHONY: dev-operator-namespace
dev-operator-namespace: ## Create the operator namespace for local dev.
	kubectl create namespace metrics-operator-system --dry-run=client -o yaml | kubectl apply -f -

.PHONY: dev-basic-metric
dev-basic-metric: ## Apply the basic metric example for local dev.
	kubectl apply -f examples/basic_metric.yaml

.PHONY: dev-managed-metric
dev-managed-metric: ## Apply the managed metric example for local dev.
	kubectl apply -f examples/managed_metric.yaml

.PHONY: dev-apply-dynatrace-prod-setup
dev-apply-dynatrace-prod-setup: ## Apply the Dynatrace prod setup example.
	kubectl apply -f examples/datasink/dynatrace-prod-setup.yaml

.PHONY: dev-apply-metric-dynatrace-prod
dev-apply-metric-dynatrace-prod: ## Apply the metric using Dynatrace prod example.
	kubectl apply -f examples/datasink/metric-using-dynatrace-prod.yaml

.PHONY: dev-v1beta1-compmetric
dev-v1beta1-compmetric: ## Apply the v1beta1 compmetric example.
	kubectl apply -f examples/v1beta1/compmetric.yaml

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster. Use ignore-not-found=true to ignore errors.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy the operator to the K8s cluster.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Remove the operator from the K8s cluster.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Linting

.PHONY: lint
lint: $(GOLANGCILINT) ## Run all linters.
	$(GOLANGCILINT) config verify
	$(GOLANGCILINT) run ./...

.PHONY: lint-fix
lint-fix: ## Run all linters and auto-fix issues.
	golangci-lint run --fix

##@ Helm

.PHONY: helm-chart
helm-chart: ## Render Helm chart templates with the current version.
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/Chart.yaml.tpl > charts/$(PROJECT_FULL_NAME)/Chart.yaml
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/values.yaml.tpl > charts/$(PROJECT_FULL_NAME)/values.yaml

.PHONY: helm-install-local
helm-install-local: docker-build ## Install the Helm chart locally and load the image into the dev cluster.
	helm upgrade --install $(PROJECT_FULL_NAME) charts/$(PROJECT_FULL_NAME)/ --set image.repository=$(IMG_BASE) --set image.tag=$(IMG_VERSION) --set image.pullPolicy=Never
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_FULL_NAME)-dev

.PHONY: helm-work
helm-work: dev-kind crossplane-install helm-install-local ## Full Helm workflow: dev cluster, Crossplane, and Helm install.
	echo "Helm work done"

##@ Crossplane

.PHONY: crossplane-install
crossplane-install: ## Install Crossplane into the dev cluster.
	helm install crossplane crossplane-stable/crossplane --namespace crossplane-system --create-namespace --wait

.PHONY: crossplane-provider-install
crossplane-provider-install: ## Install the Kubernetes provider for Crossplane.
	kubectl apply -f examples/crossplane/provider.yaml -n $(CROSSPLANE_NAMESPACE)
	kubectl wait --for=condition=Healthy provider/provider-helm --timeout=1m
	kubectl apply -f examples/crossplane/provider-config.yaml -n $(CROSSPLANE_NAMESPACE)

.PHONY: crossplane-provider-sample
crossplane-provider-sample: ## Apply a sample Crossplane release.
	kubectl apply -f examples/crossplane/release.yaml -n $(CROSSPLANE_NAMESPACE)
