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
	"fmt"
	"reflect"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller")

//addToManager contains the reconciler functions and the mandatory GV for a given controller
type addToManager struct {
	function               func(manager.Manager) error
	MandatoryGroupVersions []schema.GroupVersion
}

// AddToManagerFuncs is a list of functions to add all Controllers to the Manager and the mandatory GVs
var AddToManagerFuncs []addToManager

// AddToManager adds all Controllers which have all their mandatory GVs installed to the Manager
func AddToManager(m manager.Manager, missingGVS []schema.GroupVersion) error {
	for _, a := range AddToManagerFuncs {
		if mandatoryGVSatisfied(a, missingGVS) {
			log.Info(fmt.Sprintf("Add to manager %s:", a.MandatoryGroupVersions))
			if err := a.function(m); err != nil {
				return err
			}
		}
	}

	return nil
}

//mandatoryGVSatisfied Check if the mandaottry GVs for a controller are not missing.
func mandatoryGVSatisfied(a addToManager, missingGVS []schema.GroupVersion) bool {
	if a.MandatoryGroupVersions == nil ||
		len(a.MandatoryGroupVersions) == 0 {
		return true
	}
	for _, mandatoryGV := range a.MandatoryGroupVersions {
		for _, missingGV := range missingGVS {
			if reflect.DeepEqual(mandatoryGV, missingGV) {
				return false
			}
		}
	}

	return true
}

//GetMissingGVS gets the missing GVs
func GetMissingGVS(cfg *rest.Config) (missingGVS []schema.GroupVersion, err error) {
	log.Info("Get missing GVS")

	missingGVS = make([]schema.GroupVersion, 0)
	c, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return missingGVS, err
	}

	for _, atmf := range AddToManagerFuncs {
		for _, gv := range atmf.MandatoryGroupVersions {
			err := discovery.ServerSupportsVersion(c, gv)
			if err != nil {
				log.Info(fmt.Sprintf("%s-%s is missing", gv.Group, gv.Version))
				missingGVS = append(missingGVS, gv)
			}
		}
	}

	return missingGVS, nil
}
