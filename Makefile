
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
export GOOS         = $(shell go env GOOS)
export GOARCH       = $(ARCH_TYPE)
export GOPACKAGES   = $(shell go list ./... | grep -v /vendor | grep -v /internal | grep -v /build | grep -v /test)

export PROJECT_DIR            = $(shell 'pwd')
export BUILD_DIR              = $(PROJECT_DIR)/build
export COMPONENT_SCRIPTS_PATH = $(BUILD_DIR)
export ENDPOINT_CRD_FILE      = $(PROJECT_DIR)/build/resources/multicloud_v1beta1_endpoint_crd.yaml

## WARNING: OPERATOR-SDK - IMAGE_DESCRIPTION & DOCKER_BUILD_OPTS MUST NOT CONTAIN ANY SPACES
export IMAGE_DESCRIPTION ?= RCM_Controller
export DOCKER_FILE        = $(BUILD_DIR)/Dockerfile
export DOCKER_REGISTRY   ?= quay.io
export DOCKER_NAMESPACE  ?= open-cluster-management
export DOCKER_IMAGE      ?= $(COMPONENT_NAME)
export DOCKER_BUILD_TAG  ?= latest
export DOCKER_TAG        ?= $(shell whoami)
export DOCKER_BUILD_OPTS  = --build-arg "VCS_REF=$(VCS_REF)" \
	--build-arg "VCS_URL=$(GIT_REMOTE_URL)" \
	--build-arg "IMAGE_NAME=$(DOCKER_IMAGE)" \
	--build-arg "IMAGE_DESCRIPTION=$(IMAGE_DESCRIPTION)" \
	--build-arg "ARCH_TYPE=$(ARCH_TYPE)" 

BEFORE_SCRIPT := $(shell build/before-make.sh)

-include $(shell curl -s -H 'Authorization: token ${GITHUB_TOKEN}' -H 'Accept: application/vnd.github.v4.raw' -L https://api.github.com/repos/open-cluster-management/build-harness-extensions/contents/templates/Makefile.build-harness-bootstrap -o .build-harness-bootstrap; echo .build-harness-bootstrap)


.PHONY: deps
## Download all project dependencies
deps: init component/init

.PHONY: check
## Runs a set of required checks
check: lint ossccheck copyright-check

.PHONY: test
## Runs go unit tests
test: component/test/unit

.PHONY: build
## Builds controller binary inside of an image
build:
	docker build . \
	--build-arg "REMOTE_SOURCE=." \
	--build-arg "REMOTE_SOURCE_DIR=/remote-source" \
	--build-arg "GITHUB_TOKEN=$(GITHUB_TOKEN)" \
	-t $(DOCKER_IMAGE):$(DOCKER_BUILD_TAG) \
	-f build/Dockerfile

.PHONY: copyright-check
copyright-check:
	./build/copyright-check.sh $(TRAVIS_BRANCH)

.PHONY: clean
## Clean build-harness and remove Go generated build and test files
clean::
	@rm -rf $(BUILD_DIR)/_output
	@[ "$(BUILD_HARNESS_PATH)" == '/' ] || \
	 [ "$(BUILD_HARNESS_PATH)" == '.' ] || \
	   rm -rf $(BUILD_HARNESS_PATH)

.PHONY: run
## Run the operator against the kubeconfig targeted cluster
run:
	@operator-sdk run --local --namespace="" --operator-flags="--zap-devel=true"

.PHONY: lint
## Runs linter against go files
lint:
	@echo "Running linting tool ..."
	@GOGC=25 golangci-lint run --timeout 5m

.PHONY: ossccheck
ossccheck:
	ossc --check

.PHONY: ossc
ossc:
	ossc

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

.PHONY: kind-create-cluster
kind-create-cluster:
	kind create cluster --name functional-test

.PHONY: kind-delete-cluster
kind-delete-cluster:
	kind delete cluster --name functional-test

.PHONY: install-fake-crds
install-fake-crds:
	@echo installing crds
	kubectl apply -f test/cluster-registry-crd.yaml 
	kubectl apply -f test/fake_resourceview_crd.yaml
	kubectl apply -f test/hive_v1_clusterdeployment_crd.yaml
	kubectl apply -f test/hive_v1_selectorsyncset.yaml  
	kubectl apply -f test/hive_v1_syncset.yaml 
	kubectl apply -f test/infrastructure_crd.yaml 
	@sleep 10 

.PHONY: kind-cluster-setup
kind-cluster-setup: install-fake-crds
	@echo installing fake infrastructure resource
	kubectl apply -f test/fake_infrastructure_cr.yaml

.PHONY: functional-test
functional-test:
	ginkgo -tags functional -v --slowSpecThreshold=10 test/rcm-controller-test
