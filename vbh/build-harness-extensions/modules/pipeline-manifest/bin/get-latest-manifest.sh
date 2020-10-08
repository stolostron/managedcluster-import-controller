#!/bin/sh

# Incoming variables:
#   $1 - name of the branch to get snapshots from (typically x.y-edge or x.y-stable)
#   $2 - the Z release desired (i.e. 1.0.1, 2.0.1)
#
# Required environment variable:
#   $GITHUB_TOKEN - the token.  To github.

PIPELINE_MANIFEST_LATEST_BRANCH=$1
PIPELINE_MANIFEST_LATEST_Z_RELEASE=$2

OUTPUT=$(curl -s -H "Authorization: token $GITHUB_TOKEN" "https://api.github.com/repos/open-cluster-management/pipeline/git/trees/$PIPELINE_MANIFEST_LATEST_BRANCH?recursive=1")
LATEST=`echo $OUTPUT | jq -r --arg PIPELINE_MANIFEST_LATEST_Z_RELEASE $PIPELINE_MANIFEST_LATEST_Z_RELEASE '[.tree[] | select(.path | contains("snapshots")) | select(.path | contains($PIPELINE_MANIFEST_LATEST_Z_RELEASE))] | .[-1].path'`

BASENAME=`basename $LATEST`
echo $BASENAME

$(curl -s -H "Authorization: token $GITHUB_TOKEN" -H "Accept: application/vnd.github.v3.raw" https://raw.githubusercontent.com/open-cluster-management/pipeline/$PIPELINE_MANIFEST_LATEST_BRANCH/$LATEST --output $BASENAME)

