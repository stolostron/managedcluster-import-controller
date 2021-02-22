
SHELL := /bin/bash

export BINDATA_TEMP_DIR := $(shell mktemp -d)

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
export GOOS         = $(shell go env GOOS)
export GOARCH       = $(ARCH_TYPE)
export GOPACKAGES   = $(shell go list ./... | grep -v /vendor | grep -v /internal | grep -v /build | grep -v /test)

export PROJECT_DIR            = $(shell 'pwd')
export BUILD_DIR              = $(PROJECT_DIR)/build
export COMPONENT_SCRIPTS_PATH = $(BUILD_DIR)
export KLUSTERLET_CRD_FILE      = $(PROJECT_DIR)/build/resources/agent.open-cluster-management.io_v1beta1_klusterlet_crd.yaml

export COMPONENT_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export COMPONENT_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)
export SECURITYSCANS_IMAGE_NAME ?= $(shell cat ./COMPONENT_NAME 2> /dev/null)
export SECURITYSCANS_IMAGE_VERSION ?= $(shell cat ./COMPONENT_VERSION 2> /dev/null)

## WARNING: OPERATOR-SDK - IMAGE_DESCRIPTION & DOCKER_BUILD_OPTS MUST NOT CONTAIN ANY SPACES
export IMAGE_DESCRIPTION ?= RCM_Controller
export DOCKER_FILE        = $(BUILD_DIR)/Dockerfile
export DOCKER_REGISTRY   ?= quay.io
export DOCKER_NAMESPACE  ?= open-cluster-management
export DOCKER_IMAGE      ?= $(COMPONENT_NAME)
export DOCKER_IMAGE_COVERAGE_POSTFIX ?= -coverage
export DOCKER_IMAGE_COVERAGE      ?= $(DOCKER_IMAGE)$(DOCKER_IMAGE_COVERAGE_POSTFIX)
export DOCKER_BUILD_TAG  ?= latest
export DOCKER_TAG        ?= $(shell whoami)

BEFORE_SCRIPT := $(shell build/before-make.sh)

USE_VENDORIZED_BUILD_HARNESS ?=

# ifndef USE_VENDORIZED_BUILD_HARNESS
# # -include $(shell curl -s -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/itdove/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap?branch=code_coverage -o .build-harness-bootstrap; echo .build-harness-bootstrap)
# -include $(shell curl -s -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)
# else
# -include vbh/.build-harness-vendorized
# endif

export DOCKER_BUILD_OPTS  = --build-arg VCS_REF=$(VCS_REF) \
	--build-arg VCS_URL=$(GIT_REMOTE_URL) \
	--build-arg IMAGE_NAME=$(DOCKER_IMAGE) \
	--build-arg IMAGE_DESCRIPTION=$(IMAGE_DESCRIPTION) \
	--build-arg ARCH_TYPE=$(ARCH_TYPE) \
	--build-arg REMOTE_SOURCE=. \
	--build-arg REMOTE_SOURCE_DIR=/remote-source \
	--build-arg BUILD_HARNESS_EXTENSIONS_PROJECT=${BUILD_HARNESS_EXTENSIONS_PROJECT} \
	--build-arg GITHUB_TOKEN=$(GITHUB_TOKEN)

# Only use git commands if it exists
ifdef GIT
GIT_COMMIT      = $(shell git rev-parse --short HEAD)
GIT_REMOTE_URL  = $(shell git config --get remote.origin.url)
VCS_REF     = $(if $(shell git status --porcelain),$(GIT_COMMIT)-$(BUILD_DATE),$(GIT_COMMIT))
endif

.PHONY: deps
## Download all project dependencies
deps: init component/init

.PHONY: check
## Runs a set of required checks
check: go-bindata-check

.PHONY: test
## Runs go unit tests
test: 
	@build/run-unit-tests.sh

.PHONY: go-bindata
go-bindata:
	@if which go-bindata > /dev/null; then \
		echo "##### Updating go-bindata..."; \
		cd $(mktemp -d) && GOSUMDB=off go get -u github.com/go-bindata/go-bindata/...; \
	else \
		echo "##### installing go-bindata..."; \
		cd $(mktemp -d) && GOSUMDB=off go get -u github.com/go-bindata/go-bindata/...; \
	fi
	@go-bindata --version
	go-bindata -nometadata -pkg bindata -o pkg/bindata/bindata_generated.go -prefix resources/  resources/...

.PHONY: go-bindata-check
go-bindata-check:
	@if which go-bindata > /dev/null; then \
		echo "##### Updating go-bindata..."; \
		cd $(mktemp -d) && GOSUMDB=off go get -u github.com/go-bindata/go-bindata/...; \
	else \
		echo "##### installing go-bindata..."; \
		cd $(mktemp -d) && GOSUMDB=off go get -u github.com/go-bindata/go-bindata/...; \
	fi
	@go-bindata --version
	@echo "##### go-bindata-check ####"
	@go-bindata -nometadata -pkg bindata -o $(BINDATA_TEMP_DIR)/bindata_generated.go -prefix resources/  resources/...; \
	diff $(BINDATA_TEMP_DIR)/bindata_generated.go pkg/bindata/bindata_generated.go > go-bindata.diff; \
	if [ $$? != 0 ]; then \
	  echo "#### Difference detected and saved in go-bindata.diff, run 'make go-bindata' to regenerate the bindata_generated.go"; \
	  cat go-bindata.diff; \
	  exit 1; \
	fi
	@echo "##### go-bindata-check #### Success"

## Builds controller binary
.PHONY: build
build:
	go build -o build/_output/manager -mod=mod ./cmd/manager

## Builds instructed controller binary for coverage report
.PHONY: build-coverage
build-coverage:
	go test -covermode=atomic -coverpkg=github.com/open-cluster-management/managedcluster-import-controller/pkg/... -c -tags testrunmain ./cmd/manager -o build/_output/manager-coverage

.PHONY: clean
## Clean build-harness and remove Go generated build and test files
clean:
	@rm -rf $(BUILD_DIR)/_output
	@[ "$(BUILD_HARNESS_PATH)" == '/' ] || \
	 [ "$(BUILD_HARNESS_PATH)" == '.' ] || \
	   rm -rf $(BUILD_HARNESS_PATH)

.PHONY: run
## Run the operator against the kubeconfig targeted cluster
run: go-bindata
	go run cmd/manager/main.go -v=4

.PHONY: lint
## Runs linter against go files
lint:
	@if ! which golangci-lint > /dev/null; then \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.23.6; \
	fi
	@echo "Running linting tool ..."
	@GOGC=25 golangci-lint run --timeout 5m

.PHONY: helpz
helpz:
ifndef build-harness
	$(eval MAKEFILE_LIST := Makefile build-harness/modules/go/Makefile)
endif

############################################################
# deploy section
############################################################

deploy:
	mkdir -p overlays/deploy
	cp overlays/template/kustomization.yaml overlays/deploy
	cd overlays/deploy
	kustomize build overlays/deploy | kubectl apply -f -
	rm -rf overlays/deploy

.PHONY: install-fake-crds
install-fake-crds:
	@echo installing crds
	kubectl apply -f test/functional/resources/hive_v1_clusterdeployment_crd.yaml
	kubectl apply -f test/functional/resources/hive_v1_syncset.yaml 
	kubectl apply -f test/functional/resources/infrastructure_crd.yaml 
	kubectl apply -f test/functional/resources/apiserver_crd.yaml 
	kubectl apply -f test/functional/resources/0000_00_clusters.open-cluster-management.io_managedclusters.crd.yaml
	kubectl apply -f test/functional/resources/0000_00_work.open-cluster-management.io_manifestworks.crd.yaml
	@sleep 10 

.PHONY: kind-cluster-setup
kind-cluster-setup: install-fake-crds
	@echo installing fake infrastructure resource
	kubectl apply -f test/functional/resources/fake_infrastructure_cr.yaml
	kubectl apply -f test/functional/resources/fake_apiserver_cr.yaml

.PHONY: functional-test
functional-test:
	# ginkgo -tags functional -v --focus="(.*)import-managedcluster(.*)" --slowSpecThreshold=10 test/managedcluster-import-controller-test -- -v=5
	# ginkgo -tags functional -v --slowSpecThreshold=10 --focus="(.*)approve-csr(.*)" test/functional -- -v=1
	# ginkgo -tags functional -v --slowSpecThreshold=30 --focus="import-hub/with-manifestwork" test/functional -- -v=5
	ginkgo -tags functional -v --slowSpecThreshold=30 test/functional -- -v=5

.PHONY: functional-test-full
functional-test-full: component/build-coverage
	$(SELF) component/test/functional