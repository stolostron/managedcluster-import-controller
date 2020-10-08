#!/bin/sh

# Incoming variables:
#   $1 - name of the "base" decorated manifest json file without .json sufffix (should exist)
#   $2 - name of the "delta" decorated manifests json file without .json sufffix (can be blank)
#   $3 - name of the snapshot multiclusterhub-operator image tag (should exist; i.e. 2.1.0-SNAPSHOT-2020-08-12-06-21-22)
#   $4 - name of the snapshot endpoint-operator image tag (should exist; i.e. 2.1.0-SNAPSHOT-2020-08-12-06-21-22)
#   $5 - name of the delta image tags (to be created; i.e. 2.1.0-DEVELOPER-2020-08-12-06-21-22)
#   $6 - Z release version (i.e. 1.0.1, 2.0.1)
#   $7 - Release version (i.e. 2.0, 2.1)
#
# Requires:
#   $GITHUB_USER - GitHub user (needs access to open-cluster-management stuffs)
#   $GITHUB_TOKEN - GitHub API token
#   $DOCKER_USER - docker/quay userid
#   $DOCKER_PASS - docker/quay password
#   $QUAY_TOKEN - you know, the token... to quay (needs to be able to write index-related stuffs)

base_file=$1
delta_file=$2
PIPELINE_MANIFEST_INCOMING_MCHO_TAG=$3
PIPELINE_MANIFEST_INCOMING_EO_TAG=$4
PIPELINE_MANIFEST_NEW_INDEX_TAG=$5
Z_RELEASE_VERSION=$6
RELEASE_VERSION=$7

# For each component in the delta manifest file, replace what's in the base manifest file
for k in $(jq -c '.[]' $delta_file.json); do
    name=$(echo "$k" | jq -r '.["image-name"]')
    version=$(echo "$k" | jq -r '.["image-version"]')
    tag=$(echo "$k" | jq -r '.["image-tag"]')
    sha256=$(echo "$k" | jq -r '.["git-sha256"]')
    repository=$(echo "$k" | jq -r '.["git-repository"]')
    remote=$(echo "$k" | jq -r '.["image-remote"]')
    digest=$(echo "$k" | jq -r '.["image-digest"]')
    key=$(echo "$k" | jq -r '.["image-key"]')
    echo Replacing: $name

    # Delete the delta component from the base manifest file
    eval "make pipeline-manifest/_delete DELETED_COMPONENT=$name PIPELINE_MANIFEST_FILE_NAME=$base_file PIPELINE_MANIFEST_DIR=."

    # Add the delta component from the delta manifest file to the base manifest file
    json_string='.[. | length] |= . + {"image-name": "'$name'", "image-version": "'$version'", "image-tag": "'$tag'", "git-sha256": "'$sha256'", "git-repository": "'$repository'",  "image-remote": "'$remote'","image-digest": "'$digest'","image-key": "'$key'"}'
    json_string="'"$json_string"'"
    eval "jq $json_string $base_file.json > tmp; mv tmp $base_file.json"
    eval "make pipeline-manifest/_sort PIPELINE_MANIFEST_FILE_NAME=$base_file PIPELINE_MANIFEST_DIR=."
done

# Image remanipulation section
EO_JQUERY="jq -r '(.[] | select (.[\"image-name\"] == \"endpoint-operator\") | .[\"image-version\"])' $base_file.json"
EO_RELEASE=`eval $EO_JQUERY`
MCHO_JQUERY="jq -r '(.[] | select (.[\"image-name\"] == \"multiclusterhub-operator\") | .[\"image-version\"])' $base_file.json"
MCHO_RELEASE=`eval $MCHO_JQUERY`

echo EO_RELEASE: $EO_RELEASE
echo MCHO_RELEASE: $MCHO_RELEASE

eval 'sed -e "s:%%EO_INPUT_TAG%%:$PIPELINE_MANIFEST_INCOMING_EO_TAG:g;" -e "s:%%MANIFEST%%:$base_file:g;" -e "s:%%Z_RELEASE_VERSION%%:$Z_RELEASE_VERSION:g;" $BUILD_HARNESS_EXTENSIONS_PATH/modules/pipeline-manifest/lib/Dockerfile.eo_template > Dockerfile.eo'
eval 'sed -e "s:%%MCHO_INPUT_TAG%%:$PIPELINE_MANIFEST_INCOMING_MCHO_TAG:g;" -e "s:%%MANIFEST%%:$base_file:g;" -e "s:%%Z_RELEASE_VERSION%%:$Z_RELEASE_VERSION:g;" $BUILD_HARNESS_EXTENSIONS_PATH/modules/pipeline-manifest/lib/Dockerfile.mcho_template > Dockerfile.mcho'

#
# First build the endpoint operator so that its sha can be included in the manifest layered into the hub operator
#
eval 'docker login -u=$DOCKER_USER -p=$DOCKER_PASS quay.io'
eval 'docker pull quay.io/open-cluster-management/endpoint-operator:$PIPELINE_MANIFEST_INCOMING_EO_TAG'
eval 'docker build . -f Dockerfile.eo -t quay.io/open-cluster-management/endpoint-operator:$PIPELINE_MANIFEST_NEW_INDEX_TAG'
eval 'docker push quay.io/open-cluster-management/endpoint-operator:$PIPELINE_MANIFEST_NEW_INDEX_TAG'
ep_quaysha=`make -s retag/getquaysha RETAG_QUAY_COMPONENT_TAG=$PIPELINE_MANIFEST_NEW_INDEX_TAG COMPONENT_NAME=endpoint-operator`
echo endpoint-operator image sha: $ep_quaysha

jq --arg ep_quaysha $ep_quaysha '(.[] | select (.["image-name"] == "endpoint-operator") | .["image-digest"]) |= $ep_quaysha' $base_file.json > tmp.json ; mv tmp.json $base_file.json
if [[ "$ep_quaysha" == "null" ]]; then echo Oh no - the endpoint operator image digest is missing!; exit 1; fi

eval 'docker pull quay.io/open-cluster-management/multiclusterhub-operator:$PIPELINE_MANIFEST_INCOMING_MCHO_TAG'
eval 'docker build . -f Dockerfile.mcho -t quay.io/open-cluster-management/multiclusterhub-operator:$PIPELINE_MANIFEST_NEW_INDEX_TAG'
eval 'docker push quay.io/open-cluster-management/multiclusterhub-operator:$PIPELINE_MANIFEST_NEW_INDEX_TAG'
mco_quaysha=`make -s retag/getquaysha RETAG_QUAY_COMPONENT_TAG=$PIPELINE_MANIFEST_NEW_INDEX_TAG COMPONENT_NAME=multiclusterhub-operator`
echo multiclusterhub-operator image sha: $mco_quaysha

jq --arg mco_quaysha $mco_quaysha '(.[] | select (.["image-name"] == "multiclusterhub-operator") | .["image-digest"]) |= $mco_quaysha' $base_file.json > tmp.json ; mv tmp.json $base_file.json
if [[ "$mco_quaysha" == "null" ]]; then echo Oh no - the multiclusterhub operator image digest is missing!; exit 1; fi

# Build an index image using the release repo
if [ -d release ];  \
then cd release; git pull; cd ..; \
else git clone -b release-$RELEASE_VERSION https://${GITHUB_USER}:${GITHUB_TOKEN}@github.com/open-cluster-management/release.git release ;\
fi

# Copy over the manifest
cp $base_file.json release/image-manifests/$Z_RELEASE_VERSION.json

cd release
tools/build/build-acm-bundle-image-and-catalog.sh -P -t $PIPELINE_MANIFEST_NEW_INDEX_TAG $Z_RELEASE_VERSION
tools/build/build-ocm-hub-bundle-image-and-catalog.sh -P -t $PIPELINE_MANIFEST_NEW_INDEX_TAG $Z_RELEASE_VERSION
