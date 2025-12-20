SHELL :=/bin/bash

all: build
.PHONY: all

# Include the library makefile
include $(addprefix ./vendor/github.com/openshift/build-machinery-go/make/, \
	golang.mk \
	targets/openshift/deps.mk \
	targets/openshift/crd-schema-gen.mk \
)

GO_PACKAGES :=$(addsuffix ...,$(addprefix ./,$(filter-out test/, $(filter-out vendor/,$(filter-out hack/,$(wildcard */))))))
GO_BUILD_PACKAGES :=$(GO_PACKAGES)
GO_BUILD_PACKAGES_EXPANDED :=$(GO_BUILD_PACKAGES)
# LDFLAGS are not needed for dummy builds (saving time on calling git commands)
GO_LD_FLAGS:=

# controller-gen setup
CONTROLLER_GEN_VERSION :=v0.17.3
CONTROLLER_GEN :=$(PERMANENT_TMP_GOPATH)/bin/controller-gen
ifneq "" "$(wildcard $(CONTROLLER_GEN))"
_controller_gen_installed_version = $(shell $(CONTROLLER_GEN) --version | awk '{print $$2}')
endif
controller_gen_dir :=$(abspath $(PERMANENT_TMP_GOPATH)/bin)

# override ensure-controller-gen
ensure-controller-gen:
ifeq "" "$(wildcard $(CONTROLLER_GEN))"
	$(info Installing controller-gen into '$(CONTROLLER_GEN)')
	mkdir -p '$(controller_gen_dir)'
	GOBIN='$(controller_gen_dir)' go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_GEN_VERSION)
	chmod +x '$(CONTROLLER_GEN)';
else
	$(info Using existing controller-gen from "$(CONTROLLER_GEN)")
	@[[ "$(_controller_gen_installed_version)" == $(CONTROLLER_GEN_VERSION) ]] || \
	echo "Warning: Installed controller-gen version $(_controller_gen_installed_version) does not match expected version $(CONTROLLER_GEN_VERSION)."
endif

# $1 - target name
# $2 - apis
# $3 - manifests
# $4 - output
$(call add-crd-gen,actionv1beta1,./action/v1beta1,./action/v1beta1,./action/v1beta1)
$(call add-crd-gen,viewv1beta1,./view/v1beta1,./view/v1beta1,./view/v1beta1)
$(call add-crd-gen,clusterinfov1beta1,./clusterinfo/v1beta1,./clusterinfo/v1beta1,./clusterinfo/v1beta1)
$(call add-crd-gen,imageregistryv1beta1,./imageregistry/v1alpha1,./imageregistry/v1alpha1,./imageregistry/v1alpha1)
$(call add-crd-gen,klusterletconfigv1alpha1,./klusterletconfig/v1alpha1,./klusterletconfig/v1alpha1,./klusterletconfig/v1alpha1)

RUNTIME ?= podman
RUNTIME_IMAGE_NAME ?= openshift-api-generator

verify-scripts:
	bash -x hack/verify-deepcopy.sh
	bash -x hack/verify-swagger-docs.sh
	bash -x hack/verify-crds.sh
	bash -x hack/verify-codegen.sh
.PHONY: verify-scripts
verify: verify-scripts

update-scripts:
	hack/update-deepcopy.sh
	hack/update-swagger-docs.sh
	hack/update-codegen.sh
.PHONY: update-scripts
update: update-scripts
