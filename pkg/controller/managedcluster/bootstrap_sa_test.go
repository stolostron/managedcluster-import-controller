//Package managedcluster ...
package managedcluster

import (
	"reflect"
	"testing"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func Test_bootstrapServiceAccountNsN(t *testing.T) {
	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testdeploymentcluster",
		},
	}

	type args struct {
		cluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				cluster: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				cluster: testManagedCluster,
			},
			want: types.NamespacedName{
				Name:      "testdeploymentcluster" + bootstrapServiceAccountNamePostfix,
				Namespace: "testdeploymentcluster",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			got, err := bootstrapServiceAccountNsN(tt.args.cluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("bootstrapServiceAccountNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bootstrapServiceAccountNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}
