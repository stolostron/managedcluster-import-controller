//Package clusterimport ...
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
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
