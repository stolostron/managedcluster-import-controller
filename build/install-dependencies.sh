#!/bin/bash -e
###############################################################################
# (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
# Note to U.S. Government Users Restricted Rights:
# U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
# Contract with IBM Corp.
# Copyright (c) Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

export GO111MODULE=off

# Go tools

if ! which patter > /dev/null; then      echo "Installing patter ..."; pushd $(mktemp -d) && GOSUMDB=off go get -u github.com/apg/patter && popd; fi
if ! which gocovmerge > /dev/null; then  echo "Installing gocovmerge..."; pushd $(mktemp -d) && GOSUMDB=off go get -u github.com/wadey/gocovmerge; && popd; fi
if ! which golangci-lint > /dev/null; then
   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.23.6
fi
if ! which go-bindata > /dev/null; then
	echo "Installing go-bindata..."
	cd $(mktemp -d) && GOSUMDB=off go get -u github.com/go-bindata/go-bindata/...
fi

# Build tools

# Image tools

# Check tools
