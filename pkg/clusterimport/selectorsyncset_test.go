// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package clusterimport ...
package clusterimport

import (
	"testing"

	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/stretchr/testify/assert"
)

func TestHasClusterManagedLabels(t *testing.T) {
	clusterDeployment := &hivev1.ClusterDeployment{}

	// nil labels should return false
	t.Run("nil labels", func(t *testing.T) {
		ok := HasClusterManagedLabels(clusterDeployment)
		assert.Equal(t, ok, false, "nil labels should return false")
	})
	last := false
	// after adding label should return true, and has the right label value
	t.Run("After adding", func(t *testing.T) {
		temp := AddClusterManagedLabels(clusterDeployment)
		ok := HasClusterManagedLabels(clusterDeployment)
		assert.Equal(t, ok, last, "Should not modify original item's labels")
		ok = HasClusterManagedLabels(temp)
		assert.Equal(t, ok, true, "Should return true")
		clusterDeployment = temp
		last = true
	})
	// after removing label should return false
	t.Run("After removing", func(t *testing.T) {
		temp := RemoveClusterManagedLabels(clusterDeployment)
		ok := HasClusterManagedLabels(clusterDeployment)
		assert.Equal(t, ok, last, "Should not modify original item's labels")
		ok = HasClusterManagedLabels(temp)
		assert.Equal(t, ok, false, "Should return false")
		clusterDeployment = temp
	})

}
