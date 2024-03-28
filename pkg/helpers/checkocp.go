package helpers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

var projectGVR = schema.GroupVersionResource{Group: "project.openshift.io", Version: "v1", Resource: "projects"}

var globalDeployOnOCP bool = true

func CheckDeployOnOCP(discoveryClient discovery.DiscoveryInterface) error {
	_, err := discoveryClient.ServerResourcesForGroupVersion(projectGVR.GroupVersion().String())
	if err == nil {
		globalDeployOnOCP = true
		fmt.Println("The controller is deployed on OCP cluster.")
		return nil
	}

	if errors.IsNotFound(err) {
		globalDeployOnOCP = false
		fmt.Println("The controller is deployed on non-OCP cluster.")
		return nil
	}

	return err
}

func DeployOnOCP() bool {
	return globalDeployOnOCP
}
