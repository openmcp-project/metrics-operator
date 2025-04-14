

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
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

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
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go start

.PHONY: build-docker-binary
build-docker-binary: manifests generate fmt vet ## Build manager binary.
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o bin/manager-linux.amd64 cmd/main.go


# If you wish built the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64 ). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: build-docker-binary test ## Build docker image with the manager.
	$(CONTAINER_TOOL) build -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	$(CONTAINER_TOOL) push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> then the export will fail)
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- $(CONTAINER_TOOL) buildx create --name project-v3-builder
	$(CONTAINER_TOOL) buildx use project-v3-builder
	- $(CONTAINER_TOOL) buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross .
	- $(CONTAINER_TOOL) buildx rm project-v3-builder
	rm Dockerfile.cross

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

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


### ------------------------------------ DEVELOPMENT - LOCAL ------------------------------------ ###

.PHONY: dev-all
dev-all-deploy:
	$(MAKE) dev-deploy
	$(MAKE) crossplane-install
	$(MAKE) crossplane-provider-install
	$(MAKE) crossplane-provider-sample


.PHONY: dev-deploy
dev-deploy: manifests kustomize dev-clean
	$(KIND) create cluster --name=$(PROJECT_NAME)-dev
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_NAME)-dev


.PHONY: dev-build
dev-build: docker-build
	@echo "Finished building docker image" ${IMG}

.PHONY: dev-base
dev-base: manifests kustomize dev-build dev-clean dev-cluster helm-install-local

.PHONY: dev-cluster
dev-cluster:
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-local
dev-local:
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(MAKE) install

.PHONY: dev-local-all
dev-local-all:
	$(MAKE) dev-clean
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev
	$(MAKE) install
	$(MAKE) crossplane-install
	$(MAKE) crossplane-provider-install
	$(MAKE) crossplane-provider-sample
	$(MAKE) dev-namespace
	$(MAKE) dev-secret
	$(MAKE) dev-basic-metric
	$(MAKE) dev-managed-metric
	$(MAKE) dev-v1beta1-singlemetric
	$(MAKE) dev-v1beta1-compmetric





.PHONY: dev-secret
dev-secret:
	kubectl apply -f examples/secret.yaml

.PHONY: dev-namespace
dev-namespace:
	kubectl apply -f examples/namespace.yaml

.PHONY: dev-basic-metric
dev-basic-metric:
	kubectl apply -f examples/basic_metric.yaml

.PHONY: dev-managed-metric
dev-managed-metric:
	kubectl apply -f examples/managed_metric.yaml


.PHONY: dev-v1beta1-singlemetric
dev-v1beta1-singlemetric:
	kubectl apply -f examples/v1beta1/singlemetric.yaml

.PHONY: dev-v1beta1-compmetric
dev-v1beta1-compmetric:
	kubectl apply -f examples/v1beta1/compmetric.yaml


.PHONY: dev-kind
dev-kind:
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-clean
dev-clean:
	$(KIND) delete cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-run
dev-run:
	## todo: add flag --debug
	go run ./cmd/main.go

$(GOLANGCILINT): $(LOCALBIN)
	@if test -x $(LOCALBIN)/golangci-lint && ! $(LOCALBIN)/golangci-lint version | grep -q $(GOLANGCILINT_VERSION); then \
		echo "$(LOCALBIN)/golangci-lint version is not expected $(GOLANGCILINT_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/golangci-lint; \
	fi
	test -s $(LOCALBIN)/golangci-lint || GOBIN=$(LOCALBIN) GO111MODULE=on go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCILINT_VERSION)


.PHONY: lint
lint: $(GOLANGCILINT)
	$(GOLANGCILINT) config verify
	$(GOLANGCILINT) run ./...

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix

### ------------------------------------ HELM ------------------------------------ ###


.PHONY: helm-chart
helm-chart:
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/Chart.yaml.tpl > charts/$(PROJECT_FULL_NAME)/Chart.yaml
	OPERATOR_VERSION=$(shell cat VERSION) envsubst < charts/$(PROJECT_FULL_NAME)/values.yaml.tpl > charts/$(PROJECT_FULL_NAME)/values.yaml

.PHONY: helm-install-local
helm-install-local: docker-build
	helm upgrade --install $(PROJECT_FULL_NAME) charts/$(PROJECT_FULL_NAME)/ --set image.repository=$(IMG_BASE) --set image.tag=$(IMG_VERSION) --set image.pullPolicy=Never
	$(KIND) load docker-image ${IMG} --name=$(PROJECT_FULL_NAME)-dev



.PHONY: helm-work
helm-work: dev-kind crossplane-install helm-install-local
	echo "Helm work done"

# initializes pre-commit hooks using lefthook https://github.com/evilmartians/lefthook
lefthook:
	lefthook install

# ensure go generate doesn't create a diff
check-diff: generate manifests
	@echo checking clean branch
	@if git status --porcelain | grep . ; then echo Uncommitted changes found after running make generate manifests. Please ensure you commit all generated files in this branch after running make generate. && false; else echo branch is clean; fi

reviewable:
	@$(MAKE) generate
	@$(MAKE) lint
	@$(MAKE) test
### ------------------------------------ CROSSPLANE ------------------------------------ ###

# Namespace where Crossplane is installed
CROSSPLANE_NAMESPACE ?= crossplane-system

.PHONY: crossplane-install
crossplane-install:
	helm install crossplane crossplane-stable/crossplane --namespace crossplane-system --create-namespace --wait

# Install the Kubernetes provider using kubectl
crossplane-provider-install:
	kubectl apply -f examples/crossplane/provider.yaml -n $(CROSSPLANE_NAMESPACE)
	kubectl wait --for=condition=Healthy provider/provider-helm --timeout=1m
	kubectl apply -f examples/crossplane/provider-config.yaml -n $(CROSSPLANE_NAMESPACE)



.PHONY: install-k8s-provider

.PHONY: helm-provider-sample
crossplane-provider-sample:
	kubectl apply -f examples/crossplane/release.yaml -n $(CROSSPLANE_NAMESPACE)
