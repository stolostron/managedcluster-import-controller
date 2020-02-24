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

package controller

import (
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"

	"github.com/open-cluster-management/rcm-controller/pkg/controller/clusterdeployment"
)

func init() {
	// AddToManagerFuncs is a list of functions and manadatory GVs to create controllers and add them to a manager.
	AddToManagerFuncs = append(AddToManagerFuncs, addToManager{
		function: clusterdeployment.Add,
		MandatoryGroupVersions: []schema.GroupVersion{
			hivev1.SchemeGroupVersion,
			clusterregistryv1alpha1.SchemeGroupVersion,
		},
	})
}
