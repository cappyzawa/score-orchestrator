# Kubernetes Runtime Controller Makefile
# This Makefile is specific to the Kubernetes Runtime Controller

# Image URL to use for building/pushing
RUNTIME_IMG ?= kubernetes-runtime:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nKubernetes Runtime Controller\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: fmt
fmt: ## Run go fmt against code.
	cd ../../ && go fmt ./runtimes/kubernetes/...

.PHONY: vet
vet: ## Run go vet against code.
	cd ../../ && go vet ./runtimes/kubernetes/...

.PHONY: test
test: fmt vet ## Run tests.
	cd ../../ && go test ./runtimes/kubernetes/... -coverprofile cover.out

##@ Build

.PHONY: build
build: fmt vet ## Build kubernetes-runtime binary.
	cd ../../ && go build -o bin/kubernetes-runtime runtimes/kubernetes/cmd/main.go

.PHONY: run
run: fmt vet ## Run the kubernetes-runtime controller from your host.
	cd ../../ && go run runtimes/kubernetes/cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the kubernetes-runtime.
	cd ../../ && $(CONTAINER_TOOL) build -f runtimes/kubernetes/Dockerfile -t ${RUNTIME_IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the kubernetes-runtime.
	$(CONTAINER_TOOL) push ${RUNTIME_IMG}

##@ Deployment

# KUBECTL and KUSTOMIZE need to be available
KUBECTL ?= kubectl
# Use absolute path for kustomize to avoid path issues
PROJECT_ROOT := $(shell cd ../.. && pwd)
KUSTOMIZE_BIN := $(shell if [ -f $(PROJECT_ROOT)/bin/kustomize ]; then echo "$(PROJECT_ROOT)/bin/kustomize"; else echo "kustomize"; fi)
KUSTOMIZE ?= $(KUSTOMIZE_BIN)

.PHONY: deploy
deploy: ## Deploy kubernetes-runtime controller to the K8s cluster specified in ~/.kube/config.
	cd manifests && $(KUSTOMIZE) edit set image ghcr.io/cappyzawa/score-orchestrator/kubernetes-runtime=${RUNTIME_IMG}
	$(KUSTOMIZE) build manifests | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy kubernetes-runtime controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build manifests | $(KUBECTL) delete --ignore-not-found=true -f -

.PHONY: logs
logs: ## Show kubernetes-runtime controller logs.
	$(KUBECTL) logs -l app.kubernetes.io/name=kubernetes-runtime-controller -n score-system --tail=100 -f