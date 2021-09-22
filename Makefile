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

export CGO_ENABLED  = 0
export GO111MODULE := on
export GOPATH      ?=$(shell go env GOPATH)
export GOHOSTOS    ?=$(shell go env GOHOSTOS)
export GOHOSTARCH  ?=$(shell go env GOHOSTARCH)
export GOPACKAGES   = $(shell go list ./... | grep -v /manager | grep -v /bindata  | grep -v /vendor | grep -v /internal | grep -v /build | grep -v /test )

export PROJECT_DIR            = $(shell 'pwd')
export BUILD_DIR              = $(PROJECT_DIR)/build

export COMPONENT_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export COMPONENT_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)
export SECURITYSCANS_IMAGE_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export SECURITYSCANS_IMAGE_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)

export DOCKER_FILE        = $(BUILD_DIR)/Dockerfile
export DOCKER_IMAGE      ?= $(COMPONENT_NAME)
export DOCKER_BUILDER    ?= docker

BEFORE_SCRIPT := $(shell build/before-make.sh)

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
	go build -o build/_output/manager -mod=mod ./cmd/manager

## Builds controller image
.PHONY: build-image
build-image:
	$(DOCKER_BUILDER) build -f $(DOCKER_FILE) . -t $(DOCKER_IMAGE) 

## Builds controller image with coverage
.PHONY: build-image-coverage
build-image-coverage: build-image
	$(DOCKER_BUILDER) build -f $(DOCKER_FILE)-coverage . -t managedcluster-import-controller-coverage \
		--build-arg DOCKER_BASE_IMAGE=$(DOCKER_IMAGE) --build-arg HOST_UID=$(shell id -u)

## Clean build-harness and remove test files
.PHONY: clean
clean: clean-e2e-test
	@rm -rf _output

## Deploy the controller
.PHONY: deploy
deploy:
	kubectl apply -k deploy/base

## Build e2e test binary
.PHONY: build-e2e-test
build-e2e-test:
	go test -c ./test/e2e -o _output/e2e.test

## Runs e2e test
.PHONY: e2e-test
e2e-test: build-image-coverage build-e2e-test
	@build/run-e2e-tests.sh

## Clean e2e test
.PHONY: clean-e2e-test
clean-e2e-test:
	@build/run-e2e-tests.sh clean
