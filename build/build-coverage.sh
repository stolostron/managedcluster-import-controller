#!/bin/bash -e

# PARAMETERS
# $1 - Final image name and tag to be produced
export DOCKER_IMAGE_AND_TAG=${1}
# $2 - Final image name and tag for coverage image
export DOCKER_IMAGE_COVERAGE_AND_TAG=${2}

docker build . \
$DOCKER_BUILD_OPTS \
--build-arg DOCKER_BASE_IMAGE=$DOCKER_IMAGE_AND_TAG \
-t $DOCKER_IMAGE_COVERAGE:$DOCKER_BUILD_TAG \
-f build/Dockerfile-coverage

if [ ! -z "$DOCKER_IMAGE_COVERAGE_AND_TAG" ]; then
    echo "Retagging image as $DOCKER_IMAGE_COVERAGE_AND_TAG"
    docker tag $DOCKER_IMAGE_COVERAGE:$DOCKER_BUILD_TAG "$DOCKER_IMAGE_COVERAGE_AND_TAG"
fi