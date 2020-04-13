#!/bin/bash -e
###############################################################################
# (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
# Note to U.S. Government Users Restricted Rights:
# U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
# Contract with IBM Corp.
# Licensed Materials - Property of IBM
# Copyright (c) 2020 Red Hat, Inc.
###############################################################################

export GO111MODULE=off

# Go tools

if ! which patter > /dev/null; then      echo "Installing patter ..."; go get -u github.com/apg/patter; fi
if ! which gocovmerge > /dev/null; then  echo "Installing gocovmerge..."; go get -u github.com/wadey/gocovmerge; fi
if ! which golangci-lint > /dev/null; then
    curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.23.6
fi
if ! which ossc > /dev/null; then
	# do a get in a tmp dir to avoid local go.mod update
	cd $(mktemp -d) && GOSUMDB=off go get -u github.com/open-cluster-management/go-ossc/ossc
fi

# Build tools

if ! which operator-sdk > /dev/null; then
    OPERATOR_SDK_VER=v0.15.1
    curr_dir=$(pwd)
    echo ">>> Installing Operator SDK"
    echo ">>> >>> Downloading source code"
    set +e
    # cannot use 'set -e' because this command always fails after project has been cloned down for some reason
    go get -d github.com/operator-framework/operator-sdk
    set -e
    cd $GOPATH/src/github.com/operator-framework/operator-sdk
    echo ">>> >>> Checking out $OPERATOR_SDK_VER"
    git checkout $OPERATOR_SDK_VER
    echo ">>> >>> Running make tidy"
    GO111MODULE=on make tidy
    echo ">>> >>> Running make install"
    GO111MODULE=on make install
    echo ">>> Done installing Operator SDK"
    operator-sdk version
    cd $curr_dir
fi


# Image tools


# Check tools
