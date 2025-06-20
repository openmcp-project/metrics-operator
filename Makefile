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

.PHONY: all
all: build

##@ General
# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

#----------------------------------------------------------------------------------------------
##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUBECTL ?= kubectl
KIND ?= kind # fix this to use tools
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOTESTSUM ?= $(LOCALBIN)/gotestsum
GOLANGCILINT ?= $(LOCALBIN)/golangci-lint
HELM ?= $(LOCALBIN)/helm

## Tool Versions
KUSTOMIZE_VERSION ?= v5.4.1
CONTROLLER_TOOLS_VERSION ?= v0.17.2
GOLANGCILINT_VERSION ?= v2.0.2

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

$(GOLANGCILINT): $(LOCALBIN)
	@if test -x $(LOCALBIN)/golangci-lint && ! $(LOCALBIN)/golangci-lint version | grep -q $(GOLANGCILINT_VERSION); then \
		echo "$(LOCALBIN)/golangci-lint version is not expected $(GOLANGCILINT_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/golangci-lint; \
	fi
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) GO111MODULE=on go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCILINT_VERSION)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary. If wrong version is installed, it will be overwritten.
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $(LOCALBIN)/controller-gen && $(LOCALBIN)/controller-gen --version | grep -q $(CONTROLLER_TOOLS_VERSION) || \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $(LOCALBIN)/setup-envtest || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: envtest-bins
envtest-bins: envtest ## Download envtest binaries
	$(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN)

.PHONY: gotestsum
gotestsum: $(GOTESTSUM) ## Download gotestsum locally if necessary.
$(GOTESTSUM): $(LOCALBIN)
	test -s $(LOCALBIN)/gotestsum || GOBIN=$(LOCALBIN) go install gotest.tools/gotestsum@latest

.PHONY: lint
lint: $(GOLANGCILINT) ## Run golangci-lint against code.
	$(GOLANGCILINT) config verify
	$(GOLANGCILINT) run ./...

.PHONY: lint-fix
lint-fix: $(GOLANGCILINT) ## Run golangci-lint with --fix option to automatically fix issues.
	$(GOLANGCILINT) run --fix

#----------------------------------------------------------------------------------------------
##@ Code Generation & Formatting
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

#----------------------------------------------------------------------------------------------
##@ Build
.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: build-docker-binary
build-docker-binary: manifests generate fmt vet ## Build manager binary for Docker image.
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager-linux.amd64 cmd/main.go

.PHONY: docker-build
docker-build: test build-docker-binary ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - be using containerd image store with docker. More Info: https://docs.docker.com/engine/storage/containerd/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/

PLATFORMS ?= linux/arm64,linux/amd64

.PHONY: docker-buildx
docker-buildx: test
	$(CONTAINER_TOOL) buildx build --platform=$(PLATFORMS) --tag $(IMG) --load -f Dockerfile .

#----------------------------------------------------------------------------------------------
##@ Deployment
.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

#----------------------------------------------------------------------------------------------
##@ Local Development Utilities
.PHONY: dev-build
dev-build: docker-build ## Build the Docker image for local development.
	@echo "Finished building docker image" ${IMG}

.PHONY: kind-load-image
kind-load-image: ## Load the Docker image into the local kind cluster for development.
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_FULL_NAME)-dev

.PHONY: kind-cluster
kind-cluster: ## Create a kind cluster for development (no CRDs or resources applied).
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-clean
dev-clean: ## Delete the local kind cluster used for development.
	$(KIND) delete cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: run
run: ## Run the operator locally (for debugging/development).
	## todo: add flag --debug
	OPERATOR_CONFIG_NAMESPACE=metrics-operator-system go run ./cmd/main.go start

#----------------------------------------------------------------------------------------------
##@ Testing
.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

#----------------------------------------------------------------------------------------------
##@ Testing: Run Operator outside of the cluster
.PHONY: dev-local
dev-local: ## Create a local kind cluster and install CRDs for development.
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(MAKE) install

.PHONY: dev-local-all
dev-local-all: ## Full local dev setup: clean-up, create cluster, install CRDs, Crossplane, namespaces, secrets, and sample metrics.
	$(MAKE) dev-clean
	$(MAKE) kind-cluster
	$(MAKE) install
	$(MAKE) crossplane-install
	$(MAKE) crossplane-provider-install
	$(MAKE) helm-provider-sample
	$(MAKE) dev-operator-namespace
	$(MAKE) dev-apply-dynatrace-prod-setup
	$(MAKE) dev-basic-metric
	$(MAKE) dev-managed-metric

#----------------------------------------------------------------------------------------------
##@ Testing: Run Operator inside the cluster (production scenario)

.PHONY: dev-deploy
dev-deploy: manifests kustomize dev-build dev-clean kind-cluster kind-load-image helm-install-local ## Build the Docker image, create a local kind cluster, and deploy the Operator.

#----------------------------------------------------------------------------------------------
##@ Example Resources
.PHONY: dev-operator-namespace
dev-operator-namespace: ## Create the operator namespace if it does not exist.
	kubectl create namespace metrics-operator-system --dry-run=client -o yaml | kubectl apply -f -

.PHONY: dev-basic-metric
dev-basic-metric: ## Apply the basic metric example to the cluster.
	kubectl apply -f examples/basic_metric.yaml

.PHONY: dev-managed-metric
dev-managed-metric: ## Apply the managed metric example to the cluster.
	kubectl apply -f examples/managed_metric.yaml

.PHONY: dev-apply-dynatrace-prod-setup
dev-apply-dynatrace-prod-setup: ## Apply Dynatrace production setup example to the cluster.
	kubectl apply -f examples/datasink/dynatrace-prod-setup.yaml

.PHONY: dev-apply-metric-dynatrace-prod
dev-apply-metric-dynatrace-prod: ## Apply metric using Dynatrace production example to the cluster.
	kubectl apply -f examples/datasink/metric-using-dynatrace-prod.yaml

#----------------------------------------------------------------------------------------------
##@ Helm

HELM_VERSION ?= v3.18.0
OCI_REGISTRY ?= ghcr.io/openmcp-project/charts

$(HELM): $(LOCALBIN)
	@if test -x $(LOCALBIN)/helm && ! $(LOCALBIN)/helm version --short | grep -q $(HELM_VERSION); then \
		echo "$(LOCALBIN)/helm version is not expected $(HELM_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/helm; \
	fi
	test -s $(LOCALBIN)/helm || (curl -sSL https://get.helm.sh/helm-$(HELM_VERSION)-$(shell uname | tr '[:upper:]' '[:lower:]')-amd64.tar.gz | tar xz -C /tmp && \
	mv /tmp/$(shell uname | tr '[:upper:]' '[:lower:]')-amd64/helm $(LOCALBIN)/helm && \
	chmod +x $(LOCALBIN)/helm && \
	rm -rf /tmp/$(shell uname | tr '[:upper:]' '[:lower:]')-amd64)

.PHONY: helm-package
helm-package: $(HELM) helm-chart ## Package the Helm chart.
	$(LOCALBIN)/helm package charts/$(PROJECT_FULL_NAME)/ -d ./ --version $(shell cat VERSION)

.PHONY: helm-push
helm-push: $(HELM) ## Push the Helm chart to the OCI registry.
	$(LOCALBIN)/helm push $(PROJECT_FULL_NAME)-$(shell cat VERSION).tgz oci://$(OCI_REGISTRY)

.PHONY: helm-chart
helm-chart: ## Generate Helm chart files from templates.
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/Chart.yaml.tpl > charts/$(PROJECT_FULL_NAME)/Chart.yaml
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/values.yaml.tpl > charts/$(PROJECT_FULL_NAME)/values.yaml

.PHONY: helm-install-local
helm-install-local: $(HELM) ## Install the Helm chart locally using the Docker image
	$(LOCALBIN)/helm upgrade --install $(PROJECT_FULL_NAME) charts/$(PROJECT_FULL_NAME)/ --set image.repository=$(IMG_BASE) --set image.tag=$(IMG_VERSION) --set image.pullPolicy=Never
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_FULL_NAME)-dev

#----------------------------------------------------------------------------------------------
##@ Crossplane

# CROSSPLANE_NAMESPACE defines the namespace where Crossplane will be installed.
CROSSPLANE_NAMESPACE ?= crossplane-system

.PHONY: crossplane-install
crossplane-install: ## Install Crossplane into the cluster.
	helm install crossplane crossplane-stable/crossplane --namespace $(CROSSPLANE_NAMESPACE) --create-namespace --wait

.PHONY: crossplane-provider-install
crossplane-provider-install: ## Install the Helm provider using kubectl
	kubectl apply -f examples/crossplane/provider.yaml -n $(CROSSPLANE_NAMESPACE)
	kubectl wait --for=condition=Healthy provider/provider-helm --timeout=1m
	kubectl apply -f examples/crossplane/provider-config.yaml -n $(CROSSPLANE_NAMESPACE)

.PHONY: helm-provider-sample
helm-provider-sample: ## Apply the Helm provider sample to the cluster.
	kubectl apply -f examples/crossplane/release.yaml -n $(CROSSPLANE_NAMESPACE)

#----------------------------------------------------------------------------------------------
##@ Utility
.PHONY: lefthook
lefthook: ## Initializes pre-commit hooks using lefthook https://github.com/evilmartians/lefthook
	lefthook install

.PHONY: check-diff
check-diff: generate manifests ## Ensures that go generate doesn't create a diff
	@echo checking clean branch
	@if git status --porcelain | grep . ; then echo Uncommitted changes found after running make generate manifests. Please ensure you commit all generated files in this branch after running make generate. && false; else echo branch is clean; fi

.PHONY: reviewable
reviewable: ## Ensures that the code is reviewable by running generate, lint, and test
	@$(MAKE) generate
	@$(MAKE) lint
	@$(MAKE) test
