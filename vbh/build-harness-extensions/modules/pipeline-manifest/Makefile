DOCKER := $(shell which docker 2> /dev/null)

# GITHUB_USER containing '@' char must be escaped with '%40'
GITHUB_USER := $(shell echo $(GITHUB_USER) | sed 's/@/%40/g')
GITHUB_TOKEN ?=

# May or may not be in github enterprise
PIPELINE_MANIFEST_SITE ?= github.com
# May be in RedHat main org or in some ACM specific org
PIPELINE_MANIFEST_ORG ?= open-cluster-management
# Repo name not yet confirmed
PIPELINE_MANIFEST_REPO?= pipeline
# The URL for the pipeline repo
PIPELINE_MANIFEST_GIT_URL ?= "https://$(GITHUB_USER):$(GITHUB_TOKEN)@$(PIPELINE_MANIFEST_SITE)/$(PIPELINE_MANIFEST_ORG)/$(PIPELINE_MANIFEST_REPO).git"
# Branch of release component is on, i.e release-1.0.0
PIPELINE_MANIFEST_COMPONENT_BRANCH ?= $(PIPELINE_MANIFEST_BRANCH)
# Release version of the product
PIPELINE_MANIFEST_RELEASE_VERSION ?= $(subst release-,,$(PIPELINE_MANIFEST_BRANCH))
# Release version of the product based on the branch it came from
PIPELINE_MANIFEST_SHA_RELEASE_VERSION ?= $(PIPELINE_MANIFEST_RELEASE_VERSION)
# Directory to put manifest in
PIPELINE_MANIFEST_DIR ?= $(PIPELINE_MANIFEST_REPO)
# The name of the component to modify
PIPELINE_MANIFEST_COMPONENT ?= $(COMPONENT_NAME)
# The payload to change of the component specified by PIPELINE_MANIFEST_COMPONENT
PIPELINE_MANIFEST_COMPONENT_VERSION ?= $(COMPONENT_VERSION)
# Pipeline manifest file names
PIPELINE_MANIFEST_FILE_NAME ?= manifest
PIPELINE_MANIFEST_ALIAS_FILE_NAME ?=image-alias
# Pipeline manifest snapshot folder
PIPELINE_MANIFEST_SNAPSHOT_FOLDER ?= snapshots
# The branch of the pipeline to use for quay retagging (typically quay-retag vs. quay-retag-test)
PIPELINE_MANIFEST_RETAG_BRANCH ?= quay-retag
# The branch of the pipeline to use for grabbing the latest manifest file (typically x.y-edge or x.y-stable)
PIPELINE_MANIFEST_LATEST_BRANCH ?= 2.0-edge
# The Z release of the pipeline to use for grabbing the latest manifest file
PIPELINE_MANIFEST_LATEST_Z_RELEASE ?= 2.0.0
PIPELINE_MANIFEST_REMOTE_REPO ?= quay.io/open-cluster-management

PIPELINE_MANIFEST_COMPONENT_SUFFIX ?= $(PIPELINE_MANIFEST_COMPONENT_SHA256)

PIPELINE_MANIFEST_COMPONENT_TAG ?= $(PIPELINE_MANIFEST_COMPONENT_VERSION)-$(PIPELINE_MANIFEST_COMPONENT_SUFFIX)

PIPELINE_STAGE ?= integration

PM_MANIFEST_QUERY ?= .[] |select(.["image-name"] == "$(PIPELINE_MANIFEST_COMPONENT)")
PM_IMAGE_REMOTE_SRC_QUERY ?= .[] |select(.["image-name"] == "$(PIPELINE_MANIFEST_COMPONENT)") | .["image-remote-src"]
PM_IMAGE_VERSION_QUERY ?= .[] |select(.["image-name"] == "$(PIPELINE_MANIFEST_COMPONENT)") | .["image-version"]
PM_GIT_SHA_QUERY ?= .[] |select(.["image-name"] == "$(PIPELINE_MANIFEST_COMPONENT)") | .["git-sha256"]
PM_ADDITION_QUERY ?= .[. | length] |= . + {"image-name": "$(PIPELINE_MANIFEST_COMPONENT)", "image-version": "$(PIPELINE_MANIFEST_COMPONENT_VERSION)", "image-tag": "$(PIPELINE_MANIFEST_COMPONENT_TAG)", "git-sha256": "$(PIPELINE_MANIFEST_COMPONENT_SHA256)", "git-repository": "$(PIPELINE_MANIFEST_COMPONENT_REPO)",  "image-remote": "$(PIPELINE_MANIFEST_REMOTE_REPO)"}
PM_DELETION_QUERY ?= .[] | select(.["image-name"] != "$(DELETED_COMPONENT)")
PM_SORT_QUERY ?= . | sort_by(.["image-name"])

PROMOTE_FROM ?= $(PIPELINE_STAGE)
PROMOTE_TO ?=

REPLACED_COMPONENT ?=
DELETED_COMPONENT ?=

PIPELINE_MANIFEST_GIT_BRANCH ?= $(PIPELINE_MANIFEST_RELEASE_VERSION)-$(PROMOTE_FROM)
PIPELINE_MANIFEST_RETAG_REPO ?= quay

DATETIME_VAR := $(shell (date +%Y-%m-%d-%H-%M-%S))
Z_RELEASE_VERSION ?= $(shell curl -H "Authorization: token ${GITHUB_TOKEN}" -H "Accept: application/vnd.github.v3.raw" https://raw.githubusercontent.com/open-cluster-management/release/${PIPELINE_MANIFEST_BRANCH}/Z_RELEASE_VERSION)
	

.PHONY: pipeline-manifest/_datetime_gen
# Factory method for storing datetime variable
pipeline-manifest/_datetime_gen: %_datetime_gen:
	@if [ -f DATETIME ]; \
	then cat DATETIME; \
	else echo $(DATETIME_VAR) > DATETIME; \
	fi;

.PHONY: pipeline-manifest/_sort
# Parse the configured pipeline manifest repo
pipeline-manifest/_sort: %_sort:
	@$(JQ) '$(PM_SORT_QUERY)' $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json > tmp; mv tmp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json

.PHONY: pipeline-manifest/_clone
# Clone the configured pipeline manifest repo
pipeline-manifest/_clone: %_clone:
	@$(shell $(SELF) jq/install > /dev/null)
	@echo Starting _clone target in $$PWD
	@if [ -d $(PIPELINE_MANIFEST_DIR) ]; then \
	    $(SELF) -s pipeline-manifest/_pull; \
	else \
	    $(GIT) clone -b $(PIPELINE_MANIFEST_GIT_BRANCH) $(PIPELINE_MANIFEST_GIT_URL) $(PIPELINE_MANIFEST_DIR); \
	fi

.PHONY: pipeline-manifest/_pull
# Pull the configured pipeline manifest repo
pipeline-manifest/_pull: %_pull:
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) pull --quiet

.PHONY: pipeline-manifest/_push
# Push the configured pipeline manifest repo
pipeline-manifest/_push: %_push: %_clone
	@$(SELF) pipeline-manifest/_sort
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) commit -am "$(PIPELINE_GIT_MESSAGE)" --quiet
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) push --quiet
	@$(SELF) pipeline-manifest/_snapshot-staging

.PHONY: pipeline-manifest/_read
# Parse the configured pipeline manifest repo
pipeline-manifest/_read: %_read: %_clone
	@$(JQ) '$(PM_MANIFEST_QUERY)' $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json

.PHONY: pipeline-manifest/_read_alias
# Locate the component name in the alias json
pipeline-manifest/_read_alias: %_read_alias: %_clone
	@$(JQ) '$(PM_MANIFEST_QUERY)' $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json

.PHONY: pipeline-manifest/add
# Add component named $COMPONENT_NAME at version $COMPONENT_VERSION to the pipeline manifest in stage $PIPELINE_STAGE 
pipeline-manifest/add: %add: %_clone %_add
	@$(SELF) pipeline-manifest/_push PIPELINE_GIT_MESSAGE="Added $(PIPELINE_MANIFEST_COMPONENT)"

.PHONY: pipeline-manifest/_add
# Add component named $COMPONENT_NAME at version $COMPONENT_VERSION to the pipeline manifests in stage $PIPELINE_STAGE 
pipeline-manifest/_add: %_add:
	$(call assert-set,COMPONENT_NAME)
	$(call assert-set,COMPONENT_VERSION)
	$(call assert-set,PIPELINE_MANIFEST_COMPONENT_REPO)
	$(call assert-set,PIPELINE_MANIFEST_COMPONENT_SHA256)
	@$(JQ) '$(PM_ADDITION_QUERY)' $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json > tmp; mv tmp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json

.PHONY: pipeline-manifest/update
## Add or update component $COMPONENT_NAME to have version $COMPONENT_VERSION in the pipeline manifest in stage $PIPELINE_STAGE
pipeline-manifest/update: %update: %_clone %_update
	@$(SELF) pipeline-manifest/_push PIPELINE_GIT_MESSAGE="Updated $(PIPELINE_MANIFEST_COMPONENT)"
	@echo "Successfully updated ${PIPELINE_MANIFEST_COMPONENT} to ${PIPELINE_MANIFEST_COMPONENT}:${PIPELINE_MANIFEST_COMPONENT_VERSION} in https://$(PIPELINE_MANIFEST_SITE)/$(PIPELINE_MANIFEST_ORG)/$(PIPELINE_MANIFEST_REPO)#$(PIPELINE_MANIFEST_RELEASE_VERSION)-$(PIPELINE_STAGE)"

.PHONY: pipeline-manifest/_update
# Update component $COMPONENT_NAME to have version $COMPONENT_VERSION in the pipeline manifest in stage $PIPELINE_STAGE
pipeline-manifest/_update: %_update:
	$(call assert-set,COMPONENT_NAME)
	$(call assert-set,COMPONENT_VERSION)
	@if [[ -z `$(SELF) -s pipeline-manifest/_read_alias` ]]; \
	then echo "Component $(PIPELINE_MANIFEST_COMPONENT) does not have an entry in $(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json" ; echo "Failing the build." ; exit 1; \
	else echo "Component $(PIPELINE_MANIFEST_COMPONENT) has an entry in $(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json."; \
        fi
	@if [ -z "$(SELF) -s pipeline-manifest/_read" ]; \
	then $(SELF) pipeline-manifest/_add; \
	else $(SELF) pipeline-manifest/_replace REPLACED_COMPONENT=$(PIPELINE_MANIFEST_COMPONENT); \
	fi

.PHONY: pipeline-manifest/replace
## Remove component named $REPLACED_COMPONENT and add component named $COMPONENT_NAME at version $COMPONENT_VERSION in stage $PIPELINE_STAGE
pipeline-manifest/replace: %replace: %_clone %_replace
	@$(SELF) pipeline-manifest/_push PIPELINE_GIT_MESSAGE="Replaced $(REPLACED_COMPONENT) with $(PIPELINE_MANIFEST_COMPONENT)"

.PHONY: pipeline-manifest/_replace
# Remove component named $REPLACED_COMPONENT and add component named $COMPONENT_NAME at version $COMPONENT_VERSION in stage $PIPELINE_STAGE
pipeline-manifest/_replace: %_replace:
	$(call assert-set,COMPONENT_NAME)
	$(call assert-set,COMPONENT_VERSION)
	$(call assert-set,REPLACED_COMPONENT)
	@$(SELF) pipeline-manifest/_delete DELETED_COMPONENT=$(REPLACED_COMPONENT)
	@$(SELF) pipeline-manifest/_add

.PHONY: pipeline-manifest/delete
## Remove component named $DELETED_COMPONENT from $PIPELINE_STAGE's manifest pipeline
pipeline-manifest/delete: %delete: %_clone %_delete
	@$(SELF) pipeline-manifest/_push PIPELINE_GIT_MESSAGE="Deleted $(DELETED_COMPONENT)"

.PHONY: pipeline-manifest/_delete
# Remove component named $DELETED_COMPONENT from $PIPELINE_STAGE's manifest pipeline
pipeline-manifest/_delete: %_delete:
	$(call assert-set,DELETED_COMPONENT)
	@$(JQ) '[$(PM_DELETION_QUERY)]' $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json > tmp; mv tmp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json

.PHONY: pipeline-manifest/_promote
# Promote LATEST_MANIFEST from PROMOTE_FROM stage to PROMOTE_TO stage and tag git repos with release based on manifest TAG
pipeline-manifest/_promote: %_promote: %_clone
	$(call assert-set,PROMOTE_FROM)
	$(call assert-set,PROMOTE_TO)
	$(call assert-set,LATEST_MANIFEST)
	@echo Z_RELEASE_VERSION is $(Z_RELEASE_VERSION)
	@cp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_SNAPSHOT_FOLDER)/$(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json $(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) checkout $(PIPELINE_MANIFEST_RELEASE_VERSION)-$(PROMOTE_TO)
	@if [ ! -d $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME)-$(Z_RELEASE_VERSION).json ]; \
	then cp $(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME)-$(Z_RELEASE_VERSION).json && \
	cd $(PIPELINE_MANIFEST_DIR); $(GIT) add $(PIPELINE_MANIFEST_FILE_NAME)-$(Z_RELEASE_VERSION).json; \
	else cp $(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME)-$(Z_RELEASE_VERSION).json; fi
	@mv $(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_SNAPSHOT_FOLDER)/$(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) add $(PIPELINE_MANIFEST_SNAPSHOT_FOLDER)/$(LATEST_MANIFEST)-$(Z_RELEASE_VERSION).json
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) commit -am "Promote from $(PROMOTE_FROM) to $(PROMOTE_TO)" --quiet
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) push
	
.PHONY: pipeline-manifest/_osci_to_quay
# Copy image from OpenShift CI registry to quay.io
pipeline-manifest/_osci_to_quay: %_osci_to_quay: %_clone
	$(call assert-set,DOCKER_USER)
	$(call assert-set,DOCKER_PASS)
	@echo Starting _osci_to_quay target to copy OpenShift CI image to Quay
	@echo .. Getting image-remote-src field for component $(PIPELINE_MANIFEST_COMPONENT)
	@echo .... manifest file is at $(PIPELINE_MANIFEST_FILE_NAME).json
	@$(JQ) -r '$(PM_IMAGE_REMOTE_SRC_QUERY)' $(PIPELINE_MANIFEST_FILE_NAME).json > IMAGE_REMOTE_SRC
	@echo .... image-remote-src is `cat IMAGE_REMOTE_SRC`
	@if grep '^null$$' IMAGE_REMOTE_SRC ; \
	then \
 		echo .... image does not need to be copied from OpenShift CI ; \
 	else \
 		echo .... image needs to be copied from OpenShift CI ; \
		$(JQ) -r '$(PM_IMAGE_VERSION_QUERY)' $(PIPELINE_MANIFEST_FILE_NAME).json > IMAGE_VERSION ; \
		echo .... image-version is `cat IMAGE_VERSION` ; \
		$(JQ) -r '$(PM_GIT_SHA_QUERY)' $(PIPELINE_MANIFEST_FILE_NAME).json > GIT_SHA ; \
		echo .... git-sha is `cat GIT_SHA` ; \
		echo .. Copying from OpenShift CI to Quay ; \
		OSCI_REMOTE="`cat IMAGE_REMOTE_SRC`/$(PIPELINE_MANIFEST_RELEASE_VERSION):$(PIPELINE_MANIFEST_COMPONENT)" ; \
		echo .... Pulling $$OSCI_REMOTE ; \
 		$(DOCKER) pull $$OSCI_REMOTE ; \
		QUAY_REMOTE="$(PIPELINE_MANIFEST_REMOTE_REPO)/$(PIPELINE_MANIFEST_COMPONENT):`cat IMAGE_VERSION`-`cat GIT_SHA`" ; \
 		echo .... Retagging to $$QUAY_REMOTE ; \
 		$(DOCKER) tag "$$OSCI_REMOTE" "$$QUAY_REMOTE" ; \
 		echo .... Logging in to $(PIPELINE_MANIFEST_REMOTE_REPO) as $(DOCKER_USER) ; \
 		$(DOCKER) login $(PIPELINE_MANIFEST_REMOTE_REPO) -u $(DOCKER_USER) -p $(DOCKER_PASS) ; \
 		echo .... Pushing to $$QUAY_REMOTE ; \
 		$(DOCKER) push "$$QUAY_REMOTE" ; \
 	fi
	@echo .. Finished _osci_to_quay target

.PHONY: pipeline-manifest/_snapshot-staging
# Stage snapshot of current repo.
pipeline-manifest/_snapshot-staging: %_snapshot-staging: %_clone
	@$(SELF) pipeline-manifest/_datetime_gen
	@cp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json $(PIPELINE_MANIFEST_FILE_NAME).json
	@cp $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json $(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json
	@cd $(PIPELINE_MANIFEST_DIR); git checkout $(PIPELINE_MANIFEST_RETAG_BRANCH)
	@echo $(PIPELINE_MANIFEST_GIT_BRANCH) > $(PIPELINE_MANIFEST_DIR)/ORIGIN_BRANCH
	@echo $(PIPELINE_MANIFEST_RELEASE_VERSION) > $(PIPELINE_MANIFEST_DIR)/RELEASE_VERSION
	@echo $(Z_RELEASE_VERSION) > $(PIPELINE_MANIFEST_DIR)/Z_RELEASE_VERSION
	@echo $(PIPELINE_MANIFEST_COMPONENT) > $(PIPELINE_MANIFEST_DIR)/COMPONENT_NAME
	@cat DATETIME > $(PIPELINE_MANIFEST_DIR)/TAG
	@cp $(PIPELINE_MANIFEST_FILE_NAME).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_FILE_NAME).json
	@cp $(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) commit -am "Stage $(Z_RELEASE_VERSION) snapshot of $(PIPELINE_MANIFEST_COMPONENT)-$(PIPELINE_MANIFEST_COMPONENT_SUFFIX)" --quiet
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) push --quiet

.PHONY: pipeline-manifest/_snapshot
# Create snapshot of current repo.
pipeline-manifest/_snapshot: %_snapshot: %_clone %_validate-tag
	@cp $(PIPELINE_MANIFEST_FILE_NAME)-$(TAG)-$(PIPELINE_MANIFEST_SHA_RELEASE_VERSION).json $(PIPELINE_MANIFEST_DIR)/$(PIPELINE_MANIFEST_SNAPSHOT_FOLDER)
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) add $(PIPELINE_MANIFEST_SNAPSHOT_FOLDER)/$(PIPELINE_MANIFEST_FILE_NAME)-$(TAG)-$(PIPELINE_MANIFEST_SHA_RELEASE_VERSION).json
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) commit -am "Added $(PIPELINE_MANIFEST_SHA_RELEASE_VERSION) snapshot of $(TAG) for $(PIPELINE_MANIFEST_COMPONENT)" --quiet
	@cd $(PIPELINE_MANIFEST_DIR); $(GIT) push --quiet

.PHONY: pipeline-manifest/_validate-tag
# Create sha-based manifest (validates as it goes, fails if all images are not present)
pipeline-manifest/_validate-tag: %_validate-tag:
	@$(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/decorate-manifest.sh $(PIPELINE_MANIFEST_FILE_NAME).json $(PIPELINE_MANIFEST_FILE_NAME)-$(TAG)-$(PIPELINE_MANIFEST_SHA_RELEASE_VERSION).json $(PIPELINE_MANIFEST_ALIAS_FILE_NAME).json > out.tmp
	@echo .. "cat out.tmp"
	@cat out.tmp

.PHONY: pipeline-manifest/_retag
# Validate and retag all images or git repos identified in manifest
pipeline-manifest/_retag: %_retag:
	@python3 $(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/parser.py $(PIPELINE_MANIFEST_FILE_NAME).json $(TAG) true $(PIPELINE_MANIFEST_RETAG_REPO) $(PIPELINE_MANIFEST_SHA_RELEASE_VERSION)
	@python3 $(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/parser.py $(PIPELINE_MANIFEST_FILE_NAME).json $(TAG) false $(PIPELINE_MANIFEST_RETAG_REPO) $(PIPELINE_MANIFEST_SHA_RELEASE_VERSION)

.PHONY: pipeline-manifest/_get_latest_manifest
# Get the latest manifest file in the snapshots directory of the pipeline repo on the PIPELINE_MANIFEST_LATEST_BRANCH branch (i.e. x.y-edge or x.y-stable) specific to a Z-release
pipeline-manifest/_get_latest_manifest:
	@$(shell $(SELF) jq/install > /dev/null)
	@$(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/get-latest-manifest.sh $(PIPELINE_MANIFEST_LATEST_BRANCH) $(PIPELINE_MANIFEST_LATEST_Z_RELEASE)

.PHONY: pipeline-manifest/_postprocess
# Postprocessing steps
pipeline-manifest/_postprocess: %_postprocess:
	$(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/postprocess.sh $(PIPELINE_MANIFEST_FILE_NAME)-$(TAG)-$(PIPELINE_MANIFEST_SHA_RELEASE_VERSION).json $(TAG) $(ENDPOINT) $(PIPELINE_MANIFEST_SHA_RELEASE_VERSION)

.PHONY: pipeline-manifest/build-index
## Build bundle and index images out of a hybrid of decorated manifest files
pipeline-manifest/build-index: %build-index:
	@$(shell $(SELF) jq/install > /dev/null)
	$(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/build-index.sh $(PIPELINE_MANIFEST_FILE_NAME) $(PIPELINE_MANIFEST_DELTA_FILE_NAME) $(PIPELINE_MANIFEST_INCOMING_MCHO_TAG) $(PIPELINE_MANIFEST_INCOMING_EO_TAG) $(PIPELINE_MANIFEST_NEW_INDEX_TAG) $(Z_RELEASE_VERSION) $(PIPELINE_MANIFEST_RELEASE_VERSION)

.PHONY: pipeline-manifest/_mirror-index
# Mirror downstream acm-d and create custom registry/index
pipeline-manifest/_mirror-index: %_mirror-index:
	$(call assert-set,PIPELINE_MANIFEST_RELEASE_VERSION)
	$(call assert-set,PIPELINE_MANIFEST_ADVISORY_ID)
	$(call assert-set,PIPELINE_MANIFEST_MIRROR_ORG)
	$(call assert-set,PIPELINE_MANFIEST_INDEX_IMAGE_TAG)
	$(call assert-set,PIPELINE_MANIFEST_REDHAT_USER)
	$(call assert-set,PIPELINE_MANIFEST_REDHAT_TOKEN)
	@$(shell $(SELF) jq/install > /dev/null)
	$(BUILD_HARNESS_EXTENSIONS_PATH)/modules/pipeline-manifest/bin/_mirror-index.sh 
