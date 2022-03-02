// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package features

import (
	"k8s.io/component-base/featuregate"
	"k8s.io/klog/v2"
)

const (
	// Every feature gate should add method here following this template:
	//
	// // owner: @username
	// // alpha: v1.X
	// MyFeature featuregate.Feature = "MyFeature"

	// KlusterletDetachedMode will provide a detached importing worker for import-secret controller,
	// and will start a new detached controller to process cluster in detached mode importing,
	KlusterletDetachedMode featuregate.Feature = "KlusterletDetachedMode"
)

var (
	// DefaultMutableFeatureGate is made up of multiple mutable feature-gates.
	DefaultMutableFeatureGate featuregate.MutableFeatureGate = featuregate.NewFeatureGate()
)

func init() {
	if err := DefaultMutableFeatureGate.Add(defaultRegistrationFeatureGates); err != nil {
		klog.Fatalf("Unexpected error: %v", err)
	}
}

// defaultRegistrationFeatureGates consists of all known acm-importing
// feature keys.  To add a new feature, define a key for it above and
// add it here.
var defaultRegistrationFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	KlusterletDetachedMode: {Default: true, PreRelease: featuregate.Alpha},
}
