package localcluster

import (
	"testing"

	"github.com/stolostron/cluster-lifecycle-api/constants"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func TestIsClusterSelfManaged(t *testing.T) {
	tcs := []struct {
		name    string
		cluster *clusterv1.ManagedCluster
		expect  bool
	}{
		{
			name: "empty label",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			expect: false,
		},
		{
			name: "no label",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						"foo": "bar",
					},
				},
			},
			expect: false,
		},
		{
			name: "label is false",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						constants.SelfManagedClusterLabelKey: "false",
					},
				},
			},
			expect: false,
		},
		{
			name: "label is true",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						constants.SelfManagedClusterLabelKey: "true",
					},
				},
			},
			expect: true,
		},
		{
			name: "label is true with capital",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						constants.SelfManagedClusterLabelKey: "True",
					},
				},
			},
			expect: true,
		},
	}

	for _, testcase := range tcs {
		t.Run(testcase.name, func(test *testing.T) {
			result := IsClusterSelfManaged(testcase.cluster)
			if testcase.expect != result {
				t.Errorf("expect result is %t, got %t", testcase.expect, result)
			}
		})
	}
}
