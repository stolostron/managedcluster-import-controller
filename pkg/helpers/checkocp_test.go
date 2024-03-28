package helpers

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/fake"
	coretesting "k8s.io/client-go/testing"
)

func Test_CheckDeployOnOCP(t *testing.T) {
	cases := []struct {
		name                string
		discoveryClient     discovery.DiscoveryInterface
		expectedDeployOnOCP bool
	}{
		{
			"non-ocp case",
			&fake.FakeDiscovery{Fake: &coretesting.Fake{}},
			false,
		},
		{
			"ocp case",
			&fake.FakeDiscovery{
				Fake: &coretesting.Fake{
					Resources: []*metav1.APIResourceList{
						{
							GroupVersion: projectGVR.GroupVersion().String(),
							APIResources: []metav1.APIResource{
								{Name: "projects", Namespaced: false, Kind: "Project"},
							},
						},
					},
				},
			},
			true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := CheckDeployOnOCP(c.discoveryClient)
			if err != nil {
				t.Errorf("expected get no error")
			}
			if c.expectedDeployOnOCP != DeployOnOCP() {
				t.Errorf("expected deployOnOcp is %v,but got %v", c.expectedDeployOnOCP, DeployOnOCP())
			}
		})
	}

}
