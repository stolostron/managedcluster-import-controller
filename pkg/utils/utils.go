//Package utils contains common utility functions that gets call by many differerent packages
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
package utils

import (
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// FileExist return true if file exist
func FileExist(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}

	return !info.IsDir()
}

// UniqueStringSlice takes a string[] and remove the duplicate value
func UniqueStringSlice(stringSlice []string) []string {
	keys := make(map[string]bool)
	uniqueStringSlice := []string{}

	for _, entry := range stringSlice {
		if _, value := keys[entry]; !value {
			keys[entry] = true

			uniqueStringSlice = append(uniqueStringSlice, entry)
		}
	}

	return uniqueStringSlice
}

// RemoveFromStringSlice takes a string[] and remove all stringToRemove
func RemoveFromStringSlice(stringSlice []string, stringToRemove string) []string {
	for i, slice := range stringSlice {
		if slice == stringToRemove {
			stringSlice = append(stringSlice[0:i], stringSlice[i+1:]...)
			return RemoveFromStringSlice(stringSlice, stringToRemove)
		}
	}

	return stringSlice
}

// AppendIfDNE append stringToAppend to stringSlice if stringToAppend does not already exist in stringSlice
func AppendIfDNE(stringSlice []string, stringToAppend string) []string {
	toAppend := true

	for _, slice := range stringSlice {
		if slice == stringToAppend {
			toAppend = false
		}
	}

	if toAppend {
		stringSlice = append(stringSlice, stringToAppend)
	}

	return stringSlice
}

//AddFinalizer accepts cluster and adds provided finalizer to cluster
func AddFinalizer(o metav1.Object, finalizer string) {
	for _, f := range o.GetFinalizers() {
		if f == finalizer {
			return
		}
	}

	o.SetFinalizers(append(o.GetFinalizers(), finalizer))
}

//RemoveFinalizer accepts cluster and removes provided finalizer if present
func RemoveFinalizer(o metav1.Object, finalizer string) {
	var finalizers []string

	for _, f := range o.GetFinalizers() {
		if f != finalizer {
			finalizers = append(finalizers, f)
		}
	}

	if len(finalizers) == len(o.GetFinalizers()) {
		return
	}

	o.SetFinalizers(finalizers)
}
