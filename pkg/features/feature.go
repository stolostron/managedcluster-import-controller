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

	// HypershiftImport will provide a hypershift importing worker for import-secret controller,
	// and will start a new hypershift detached controller to process hypershift cluster importing,
	// and in the future this will also start a new controllers to watch HypershiftDeployment and
	// import it to the hub cluster automatically.
	HypershiftImport featuregate.Feature = "HypershiftImport"
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
	HypershiftImport: {Default: false, PreRelease: featuregate.Alpha},
}
