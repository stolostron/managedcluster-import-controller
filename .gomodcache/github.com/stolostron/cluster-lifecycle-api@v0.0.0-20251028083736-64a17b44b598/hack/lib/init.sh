#!/bin/bash

# This script is meant to be the entrypoint for OpenShift Bash scripts to import all of the support
# libraries at once in order to make Bash script preambles as minimal as possible. This script recur-
# sively `source`s *.sh files in this directory tree. As such, no files should be `source`ed outside
# of this script to ensure that we do not attempt to overwrite read-only variables.

set -o errexit
set -o nounset
set -o pipefail

API_GROUP_VERSIONS="\
action/v1beta1 \
view/v1beta1 \
clusterinfo/v1beta1 \
imageregistry/v1alpha1 \
klusterletconfig/v1alpha1 \
"

API_PACKAGES="\
github.com/stolostron/cluster-lifecycle-api/action/v1beta1,\
github.com/stolostron/cluster-lifecycle-api/view/v1beta1,\
github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1,\
github.com/stolostron/cluster-lifecycle-api/imageregistry/v1alpha1\
github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1\
"
