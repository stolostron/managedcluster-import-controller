package helpers

import (
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func Test_getClusterName(t *testing.T) {
	csrNameReconcile := "test-csr"
	clusterName := "test-cluster"
	testCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
	}

	testCSRBadLabel := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				"badLabel": clusterName,
			},
		},
	}

	testCSRNoLabel := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
		},
	}

	type args struct {
		csr *certificatesv1.CertificateSigningRequest
	}
	tests := []struct {
		name            string
		args            args
		wantClusterName string
	}{
		{
			name: "testCSR",
			args: args{
				csr: testCSR,
			},
			wantClusterName: clusterName,
		},
		{
			name: "testCSRBadLabel",
			args: args{
				csr: testCSRBadLabel,
			},
			wantClusterName: "",
		},
		{
			name: "testCSRNoLabel",
			args: args{
				csr: testCSRNoLabel,
			},
			wantClusterName: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if gotClusterName := GetClusterName(tt.args.csr); gotClusterName != tt.wantClusterName {
				t.Errorf("getClusterName() = %v, want %v", gotClusterName, tt.wantClusterName)
			}
		})
	}
}

func TestGetBootstrapSAName(t *testing.T) {
	cases := []struct {
		name           string
		clusterName    string
		expectedSAName string
		managedCluster *clusterv1.ManagedCluster
	}{
		{
			name:           "short name",
			clusterName:    "123456789",
			expectedSAName: "123456789-bootstrap-sa",
		},
		{
			name:           "long name",
			clusterName:    "123456789-123456789-123456789-123456789-123456789-123456789",
			expectedSAName: "123456789-123456789-123456789-123456789-123456789--bootstrap-sa",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.expectedSAName != GetBootstrapSAName(c.clusterName) {
				t.Errorf("expected sa %v, but got %v", c.expectedSAName, GetBootstrapSAName(c.clusterName))
			}
		})
	}
}
