# Copyright Contributors to the Open Cluster Management project


SHELL := /bin/bash

export GIT_COMMIT      = $(shell git rev-parse --short HEAD)
export GIT_REMOTE_URL  = $(shell git config --get remote.origin.url)
export GITHUB_USER    := $(shell echo $(GITHUB_USER) | sed 's/@/%40/g')
export GITHUB_TOKEN   ?=

export ARCH       ?= $(shell uname -m)
export ARCH_TYPE   = $(if $(patsubst x86_64,,$(ARCH)),$(ARCH),amd64)
export BUILD_DATE  = $(shell date +%m/%d@%H:%M:%S)
export VCS_REF     = $(if $(shell git status --porcelain),$(GIT_COMMIT)-$(BUILD_DATE),$(GIT_COMMIT))

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

# Only use git commands if it exists
ifdef GIT
GIT_COMMIT      = $(shell git rev-parse --short HEAD)
GIT_REMOTE_URL  = $(shell git config --get remote.origin.url)
VCS_REF     = $(if $(shell git status --porcelain),$(GIT_COMMIT)-$(BUILD_DATE),$(GIT_COMMIT))
endif

## Runs a set of required checks
.PHONY: check
check: check-copyright lint

.PHONY: check-copyright
check-copyright:
	@build/check-copyright.sh

.PHONY: lint
lint:
	build/run-lint-check.sh

## Runs unit tests
.PHONY: test
test:
	@build/run-unit-tests.sh

## Builds controller binary
.PHONY: build
build:
	go build -o $(BUILD_OUTPUT_DIR)/manager ./cmd/manager

## Builds controller binary with coverage
.PHONY: build-coverage
build-coverage:
	go test -covermode=atomic -coverpkg=github.com/stolostron/managedcluster-import-controller/pkg/... \
	-c -tags testrunmain ./cmd/manager -o $(BUILD_OUTPUT_DIR)/manager-coverage

## Builds controller image
.PHONY: build-image
build-image:
	$(DOCKER_BUILDER) build -f $(DOCKER_FILE) . -t $(DOCKER_IMAGE)


## Builds controller image using buildx for amd64
.PHONY: build-image-amd64
build-image-amd64:
	$(DOCKER_BUILDER) buildx build --platform linux/amd64 -f $(DOCKER_FILE) . -t $(DOCKER_IMAGE)

## Clean build-harness and remove test files
.PHONY: clean
clean: clean-e2e-test
	@rm -rf _output

## Deploy the controller
.PHONY: deploy
deploy:
	kubectl apply -k deploy/base

## Runs e2e test
.PHONY: e2e-test
e2e-test: build-image
	@build/setup-kind-clusters.sh
	@build/setup-ocm.sh
	@build/setup-import-controller.sh
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="!agent-registration"

## Clean e2e test
.PHONY: clean-e2e-test
clean-e2e-test:
	@build/setup-kind-clusters.sh clean

## Run e2e test against Prow(an OCP cluster)
.PHONY: e2e-test-prow
e2e-test-prow:
	@build/setup-prow.sh
	@build/setup-ocm.sh enable-auto-approval
	@build/setup-import-controller.sh enable-agent-registration
	go test -c ./test/e2e -o _output/e2e.test
	_output/e2e.test -test.v -ginkgo.v --ginkgo.label-filter="agent-registration"

# Update vendor
.PHONY: vendor
vendor:
	go mod tidy -compat=1.18
	go mod vendor

# Copy CRDs
.PHONY: copy-crd
copy-crd: vendor
	bash -x hack/copy-crds.sh
