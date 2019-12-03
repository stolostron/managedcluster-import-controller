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
	"testing"

	"github.com/stretchr/testify/assert"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func init() {
	os.Setenv("ENDPOINT_CRD_FILE", "../../build/resources/multicloud_v1beta1_endpoint_crd.yaml")
}

func TestUniqueStringSlice(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))

	testCases := []struct {
		Input          []string
		ExpectedOutput []string
	}{
		{[]string{"foo", "bar"}, []string{"foo", "bar"}},
		{[]string{"foo", "bar", "bar"}, []string{"foo", "bar"}},
		{[]string{"foo", "foo", "bar", "bar"}, []string{"foo", "bar"}},
	}

	for _, testCase := range testCases {
		assert.Equal(t, testCase.ExpectedOutput, UniqueStringSlice(testCase.Input))
	}
}

func TestRemoveFromStringSlice(t *testing.T) {
	testCases := []struct {
		Input          []string
		StringToRemove string
		ExpectedOutput []string
	}{
		{[]string{"foo"}, "foo", []string{}},
		{[]string{"foo", "foo"}, "foo", []string{}},
		{[]string{"foo", "foo", "foo"}, "foo", []string{}},
		{[]string{"foo", "bar", "foo"}, "foo", []string{"bar"}},
		{[]string{"foo", "bar", "foo", "bar", "foo"}, "foo", []string{"bar", "bar"}},
		{[]string{"bar"}, "foo", []string{"bar"}},
	}

	for _, testCase := range testCases {
		input := testCase.Input
		stringToRemove := testCase.StringToRemove
		output := RemoveFromStringSlice(input, stringToRemove)

		assert.Equal(t, testCase.Input, input)
		assert.Equal(t, testCase.ExpectedOutput, output)
	}
}

func TestAppendIfDNE(t *testing.T) {
	testCases := []struct {
		Input          []string
		StringToAppend string
		ExpectedOutput []string
	}{
		{[]string{"foo"}, "bar", []string{"foo", "bar"}},
		{[]string{"foo", "bar"}, "foo", []string{"foo", "bar"}},
		{[]string{"foo", "bar"}, "bar", []string{"foo", "bar"}},
		{[]string{"foo", "bar"}, "test", []string{"foo", "bar", "test"}},
	}

	for _, testCase := range testCases {
		input := testCase.Input
		stringToAppend := testCase.StringToAppend
		output := AppendIfDNE(input, stringToAppend)

		assert.Equal(t, testCase.Input, input)
		assert.Equal(t, testCase.ExpectedOutput, output)
	}
}

func TestFileExist(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{"exist", os.Getenv("ENDPOINT_CRD_FILE"), true},
		{"dne", "do_not_exist.txt", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileExist(tt.filename); got != tt.want {
				t.Errorf("FileExist() = %v, want %v", got, tt.want)
			}
		})
	}
}
