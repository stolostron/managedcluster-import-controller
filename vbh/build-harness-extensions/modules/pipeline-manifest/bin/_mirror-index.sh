#!/bin/bash

# Execute all the mechanics of creating a custom catalog
#  1. Make sure we can talk to brew to extract the downstream build contents
#  2. Check out/refresh the ashdod and release projects, required for this process
#  3. Mirror the built images
#  4. Query the production redhat docker registry to see what upgrade bundles we can add
#  5. Build our catalog and push it

# Get logged into brew, update/check out the repos we need
echo Preparing environment...
brew hello
if [ -d ashdod ];  \
    then cd ashdod; git pull --quiet;cd ..; \
    else git clone -b master git@github.com:rh-deliverypipeline/ashdod.git ashdod; \
fi
if [ -d release ];  \
    then cd release; git checkout release-$PIPELINE_MANIFEST_RELEASE_VERSION; git pull --quiet;cd ..; \
    else git clone -b release-$PIPELINE_MANIFEST_RELEASE_VERSION git@github.com:open-cluster-management/release.git release; \
fi

# Mirror the images we explicitly build
echo Mirroring main images...
cd ashdod; python3 -u ashdod/main.py --advisory_id $PIPELINE_MANIFEST_ADVISORY_ID --org $PIPELINE_MANIFEST_MIRROR_ORG | tee ../.ashdod_output; cd ..
cat .ashdod_output | grep "Image to mirror: acm-operator-bundle:" | awk -F":" '{print $3}' | tee .acm_operator_bundle_tag

# Find the prior bundles to include
echo Locating upgrade bundles...
docker login -u $PIPELINE_MANIFEST_REDHAT_USER -p $PIPELINE_MANIFEST_REDHAT_TOKEN registry.access.redhat.com
export REDHAT_REGISTRY_TOKEN=`curl --silent -u "$PIPELINE_MANIFEST_REDHAT_USER":$PIPELINE_MANIFEST_REDHAT_TOKEN "https://sso.redhat.com/auth/realms/rhcc/protocol/redhat-docker-v2/auth?service=docker-registry&client_id=curl&scope=repository:rhel:pull" | jq -r '.access_token'`
rm .extrabs
curl --silent --location -H "Authorization: Bearer $REDHAT_REGISTRY_TOKEN" https://registry.redhat.io/v2/rhacm2/acm-operator-bundle/tags/list | jq -r '[.tags[] | select(test("'$PIPELINE_MANIFEST_BUNDLE_REGEX'"))] | sort_by(.)[]'| xargs -L1 -I'{}' echo "-B registry.redhat.io/rhacm2/acm-operator-bundle:{}" >> .extrabs
export COMPUTED_UPGRADE_BUNDLES=`cat .extrabs`
echo Adding upgrade bundles:
echo $COMPUTED_UPGRADE_BUNDLES

# Build the catalog
cd release; tools/downstream-testing/build-catalog.sh `cat ../.acm_operator_bundle_tag` $PIPELINE_MANFIEST_INDEX_IMAGE_TAG; cd ..
# Try to make the droppings writable - useful for shared machines
chmod a+w -R /tmp/acm-custom-registry*
