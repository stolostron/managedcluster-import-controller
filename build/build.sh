#!/bin/bash -e

# PARAMETERS
# $1 - Final image name and tag to be produced

echo Building operator
echo GOOS: $GOOS
echo GOARCH: $GOARCH
echo "--IMAGE: $DOCKER_IMAGE"
echo "--TAG: $DOCKER_BUILD_TAG"
echo "--DOCKER_BUILD_OPTS: $DOCKER_BUILD_OPTS"
operator-sdk build $DOCKER_IMAGE:$DOCKER_BUILD_TAG --image-build-args "$DOCKER_BUILD_OPTS"

if [ ! -z "$TRAVIS" ]; then
    echo "Retagging image as $1"
    docker tag $DOCKER_IMAGE:$DOCKER_BUILD_TAG $1
fi
