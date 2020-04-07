// (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
// Note to U.S. Government Users Restricted Rights:
// U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
// Contract with IBM Corp.
// Licensed Materials - Property of IBM
//
// Copyright (c) 2020 Red Hat, Inc.

//Package utils contains common utility functions that gets call by many differerent packages
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
