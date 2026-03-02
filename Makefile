# Copyright Contributors to the Open Cluster Management project


SHELL := /bin/bash

export CGO_ENABLED  = 1
export GOFLAGS ?=
export GO111MODULE := on
export GOPATH      ?=$(shell go env GOPATH)
export GOHOSTOS    ?=$(shell go env GOHOSTOS)
export GOHOSTARCH  ?=$(shell go env GOHOSTARCH)
export GOPACKAGES   = $(shell go list ./... | grep -v /manager | grep -v /bindata  | grep -v /vendor | grep -v /internal | grep -v /build | grep -v /test )

export PROJECT_DIR            = $(shell 'pwd')
export BUILD_DIR              = $(PROJECT_DIR)/build
export BUILD_OUTPUT_DIR       ?= _output

export COMPONENT_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export COMPONENT_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)
export SECURITYSCANS_IMAGE_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export SECURITYSCANS_IMAGE_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)

export DOCKER_FILE        = $(BUILD_DIR)/Dockerfile
export DOCKER_IMAGE      ?= $(COMPONENT_NAME)
export DOCKER_BUILDER    ?= docker

# Helm
HELM_ARCHOS:=linux-amd64
ifeq ($(GOHOSTOS),darwin)
	ifeq ($(GOHOSTARCH),amd64)
		OPERATOR_SDK_ARCHOS:=darwin_amd64
		HELM_ARCHOS:=darwin-amd64
	endif
	ifeq ($(GOHOSTARCH),arm64)
		OPERATOR_SDK_ARCHOS:=darwin_arm64
		HELM_ARCHOS:=darwin-arm64
	endif
endif

HELM?=$(PWD)/_output/helm
HELM_VERSION?=v3.14.0
helm_gen_dir:=$(dir $(HELM))

## Runs a set of required checks
.PHONY: check
check: check-copyright lint

.PHONY: check-copyright
check-copyright:
	@build/check-copyright.sh

.PHONY: lint
lint:
	@bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/open-cluster-management-io/sdk-go/main/ci/lint/run-lint.sh | bash'

ENSURE_ENVTEST_SCRIPT := https://raw.githubusercontent.com/open-cluster-management-io/sdk-go/main/ci/envtest/ensure-envtest.sh

.PHONY: envtest-setup
envtest-setup:
	$(eval export KUBEBUILDER_ASSETS=$(shell curl -fsSL $(ENSURE_ENVTEST_SCRIPT) | bash))
	@echo "KUBEBUILDER_ASSETS=$(KUBEBUILDER_ASSETS)"

## Runs unit tests
.PHONY: test
test: envtest-setup
	# Workaround for Go 1.25.x build cache regression with CGO_ENABLED=1
	# See: https://github.com/golang/go/issues/69566
	go clean -cache
	mkdir -p _output/unit/coverage
	go test -cover -covermode=atomic -coverprofile=_output/unit/coverage/cover.out $(GOPACKAGES)

## Builds controller binary
.PHONY: build
build:
	go build -o $(BUILD_OUTPUT_DIR)/manager ./cmd/manager

## Builds controller image
.PHONY: build-image
build-image:
	$(DOCKER_BUILDER) build -f $(DOCKER_FILE) . -t $(DOCKER_IMAGE)

## Builds controller image using buildx for amd64
.PHONY: build-image-amd64
build-image-amd64:
	$(DOCKER_BUILDER) buildx build --platform linux/amd64 --load -f $(DOCKER_FILE) . -t $(DOCKER_IMAGE)

## Clean build-harness and remove test files
.PHONY: clean
clean: clean-e2e-test
	@rm -rf _output

## Runs e2e test
.PHONY: e2e-test
e2e-test: build-image ensure-helm
	@build/setup-kind-clusters.sh
	@build/setup-ocm.sh
	@build/setup-import-controller.sh
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="!agent-registration" --ginkgo.timeout=2h --ginkgo.fail-fast

## Runs e2e test for core functionality with single cluster
.PHONY: e2e-test-core
e2e-test-core: build-image ensure-helm
	@build/setup-kind-clusters.sh single
	@build/setup-ocm.sh
	@build/setup-import-controller.sh
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="core && !agent-registration" --ginkgo.timeout=45m --ginkgo.fail-fast

## Runs e2e test for miscellaneous tests with single cluster
.PHONY: e2e-test-misc
e2e-test-misc: build-image ensure-helm
	@build/setup-kind-clusters.sh single
	@build/setup-ocm.sh
	@build/setup-import-controller.sh
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="!core && !hosted && !agent-registration" --ginkgo.timeout=45m --ginkgo.fail-fast

## Runs e2e test for hosted tests with dual clusters
.PHONY: e2e-test-hosted
e2e-test-hosted: build-image ensure-helm
	@build/setup-kind-clusters.sh
	@build/setup-ocm.sh
	@build/setup-import-controller.sh
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="hosted" --ginkgo.timeout=45m --ginkgo.fail-fast

## Clean e2e test
.PHONY: clean-e2e-test
clean-e2e-test:
	@build/setup-kind-clusters.sh clean

## Run e2e test against Prow(an OCP cluster)
.PHONY: e2e-test-prow
e2e-test-prow: ensure-helm
	@build/setup-prow.sh
	@build/setup-ocm.sh enable-auto-approval
	@build/setup-import-controller.sh enable-agent-registration
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="agent-registration" --ginkgo.timeout=2h --ginkgo.fail-fast

ensure-helm:
ifeq "" "$(wildcard $(HELM))"
	$(info Installing helm into '$(HELM)')
	mkdir -p '$(helm_gen_dir)'
	curl -s -f -L https://get.helm.sh/helm-$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz -o '$(helm_gen_dir)$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz'
	tar -zvxf '$(helm_gen_dir)/$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz' -C $(helm_gen_dir)
	mv $(helm_gen_dir)/$(HELM_ARCHOS)/helm $(HELM)
	rm -rf $(helm_gen_dir)/$(HELM_ARCHOS)
	chmod +x '$(HELM)';
else
	$(info Using existing helm from "$(HELM)")
endif
