# Copyright Contributors to the Open Cluster Management project

SHELL := /bin/bash
.DEFAULT_GOAL := check
# Explicit targets only; skip built-in suffix rules.
.SUFFIXES:
# Do not search implicit rules to remake this file on every invocation.
Makefile: ;

export CGO_ENABLED := 1

# build/setup-kind-clusters.sh reads these from the environment.
ifeq ($(GOHOSTOS),)
GOHOSTOS := $(shell go env GOHOSTOS)
endif
ifeq ($(GOHOSTARCH),)
GOHOSTARCH := $(shell go env GOHOSTARCH)
endif
export GOHOSTOS
export GOHOSTARCH

GOPACKAGES := $(shell go list ./... | grep -Ev '/(manager|bindata|vendor|internal|build|test)')

BUILD_OUTPUT_DIR ?= _output
CONTAINER_FILE := build/Dockerfile
CONTAINER_BUILDER ?= docker
CONTAINER_IMAGE ?= $(shell cat COMPONENT_NAME)

HELM ?= $(CURDIR)/_output/helm
HELM_VERSION ?= v3.14.0
HELM_ARCHOS := $(GOHOSTOS)-$(GOHOSTARCH)
HELM_DIR := $(dir $(HELM))

ENSURE_ENVTEST_SCRIPT_REF ?= main
ENSURE_ENVTEST_SCRIPT := https://raw.githubusercontent.com/open-cluster-management-io/sdk-go/$(ENSURE_ENVTEST_SCRIPT_REF)/ci/envtest/ensure-envtest.sh
# stdout is only the KUBEBUILDER_ASSETS path; the script logs to stderr.
ENVTEST_ASSETS_CMD = bash -o pipefail -c 'curl -fsSL "$(ENSURE_ENVTEST_SCRIPT)" | bash'

# Targets set E2E_LABEL_FILTER and E2E_TIMEOUT.
define RUN_E2E
  mkdir -p $(BUILD_OUTPUT_DIR)
  go test -c ./test/e2e -o $(BUILD_OUTPUT_DIR)/e2e.test
  $(BUILD_OUTPUT_DIR)/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="$(E2E_LABEL_FILTER)" --ginkgo.timeout=$(E2E_TIMEOUT) --ginkgo.fail-fast
endef

# Targets may also set E2E_KIND_MODE, E2E_OCM_ARGS, and E2E_IMPORT_ARGS.
# Empty kind mode selects the script default (dual clusters).
define RUN_KIND_E2E
  @CONTAINER_IMAGE="$(CONTAINER_IMAGE)" build/setup-kind-clusters.sh "$(E2E_KIND_MODE)"
  @build/setup-ocm.sh "$(E2E_OCM_ARGS)"
  @IMPORT_CONTROLLER_IMAGE_NAME="$(CONTAINER_IMAGE)" build/setup-import-controller.sh "$(E2E_IMPORT_ARGS)"
  $(RUN_E2E)
endef

## Runs a set of required checks
.PHONY: check
check: check-copyright lint

.PHONY: check-copyright
check-copyright:
	@build/check-copyright.sh

.PHONY: lint
lint:
	@bash -o pipefail -c 'curl -fsSL https://raw.githubusercontent.com/open-cluster-management-io/sdk-go/main/ci/lint/run-lint.sh | bash'

## Prints the envtest asset directory
.PHONY: envtest-setup
envtest-setup:
	@KUBEBUILDER_ASSETS=$$($(ENVTEST_ASSETS_CMD)) || exit $$?; \
		echo "KUBEBUILDER_ASSETS=$${KUBEBUILDER_ASSETS}"

# Workaround for a Go 1.25.x build-cache regression with CGO_ENABLED=1.
# See: https://github.com/golang/go/issues/69566
## Runs unit tests
.PHONY: test
test:
	@mkdir -p $(BUILD_OUTPUT_DIR)/unit/coverage
	go clean -cache
	@KUBEBUILDER_ASSETS="$$($(ENVTEST_ASSETS_CMD))" \
		go test -cover -covermode=atomic -coverprofile=$(BUILD_OUTPUT_DIR)/unit/coverage/cover.out $(GOPACKAGES)

## Builds controller binary
.PHONY: build
build:
	@mkdir -p $(BUILD_OUTPUT_DIR)
	go build -o $(BUILD_OUTPUT_DIR)/manager ./cmd/manager
	go build -o $(BUILD_OUTPUT_DIR)/tls-profile-sync ./cmd/tls-profile-sync

## Builds controller image
.PHONY: build-image
build-image:
	$(CONTAINER_BUILDER) build -f $(CONTAINER_FILE) . -t $(CONTAINER_IMAGE)

## Builds controller image using buildx for amd64
.PHONY: build-image-amd64
build-image-amd64:
	$(CONTAINER_BUILDER) buildx build --platform linux/amd64 --load -f $(CONTAINER_FILE) . -t $(CONTAINER_IMAGE)

## Removes build output and kind e2e clusters
.PHONY: clean
clean: clean-e2e-test
	@rm -rf $(BUILD_OUTPUT_DIR)

## Runs e2e test
.PHONY: e2e-test
e2e-test: E2E_LABEL_FILTER := !agent-registration
e2e-test: E2E_TIMEOUT := 2h
e2e-test: build-image ensure-helm
	$(RUN_KIND_E2E)

## Runs e2e test for core functionality with single cluster
.PHONY: e2e-test-core
e2e-test-core: E2E_KIND_MODE := single
e2e-test-core: E2E_LABEL_FILTER := core && !agent-registration
e2e-test-core: E2E_TIMEOUT := 45m
e2e-test-core: build-image ensure-helm
	$(RUN_KIND_E2E)

## Runs e2e test for miscellaneous tests with single cluster
.PHONY: e2e-test-misc
e2e-test-misc: E2E_KIND_MODE := single
e2e-test-misc: E2E_LABEL_FILTER := !core && !hosted && !agent-registration
e2e-test-misc: E2E_TIMEOUT := 45m
e2e-test-misc: build-image ensure-helm
	$(RUN_KIND_E2E)

## Runs e2e test for hosted tests with dual clusters
.PHONY: e2e-test-hosted
e2e-test-hosted: E2E_LABEL_FILTER := hosted
e2e-test-hosted: E2E_TIMEOUT := 45m
e2e-test-hosted: build-image ensure-helm
	$(RUN_KIND_E2E)

## Clean e2e test
.PHONY: clean-e2e-test
clean-e2e-test:
	@build/setup-kind-clusters.sh clean

## Run e2e test against Prow(an OCP cluster)
.PHONY: e2e-test-prow
e2e-test-prow: E2E_LABEL_FILTER := agent-registration
e2e-test-prow: E2E_TIMEOUT := 2h
e2e-test-prow: ensure-helm
	@build/setup-prow.sh
	@build/setup-ocm.sh enable-auto-approval
	@build/setup-import-controller.sh enable-agent-registration
	$(RUN_E2E)

## Installs a local Helm binary when one is not already present
.PHONY: ensure-helm
ensure-helm:
ifeq ($(wildcard $(HELM)),)
	$(info Installing helm into '$(HELM)')
	mkdir -p '$(HELM_DIR)'
	curl -s -f -L https://get.helm.sh/helm-$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz -o '$(HELM_DIR)$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz'
	tar -zvxf '$(HELM_DIR)$(HELM_VERSION)-$(HELM_ARCHOS).tar.gz' -C '$(HELM_DIR)'
	mv '$(HELM_DIR)$(HELM_ARCHOS)/helm' '$(HELM)'
	rm -rf '$(HELM_DIR)$(HELM_ARCHOS)'
	chmod +x '$(HELM)'
else
	$(info Using existing helm from '$(HELM)')
	@:
endif
