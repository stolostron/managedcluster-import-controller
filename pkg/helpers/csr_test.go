package helpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

func newTestCSRRequest(t *testing.T, commonName string, orgs []string) []byte {
	t.Helper()
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	csrb, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: orgs,
		},
	}, pk)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrb})
}

func TestValidateClusterCSRRequest(t *testing.T) {
	cluster := "mycluster"
	agent := "klusterlet"
	validCN := SubjectPrefix + cluster + ":" + agent
	validOrgs := []string{SubjectPrefix + cluster, ManagedClustersGroup}

	tests := []struct {
		name        string
		clusterName string
		signerName  string
		cn          string
		orgs        []string
		want        bool
	}{
		{
			name:        "valid kube-apiserver-client csr",
			clusterName: cluster,
			signerName:  certificatesv1.KubeAPIServerClientSignerName,
			cn:          validCN,
			orgs:        validOrgs,
			want:        true,
		},
		{
			name:        "valid without managed-clusters group",
			clusterName: cluster,
			signerName:  certificatesv1.KubeAPIServerClientSignerName,
			cn:          validCN,
			orgs:        []string{SubjectPrefix + cluster},
			want:        true,
		},
		{
			name:        "valid grpc signer",
			clusterName: cluster,
			signerName:  GRPCAuthSigner,
			cn:          validCN,
			orgs:        validOrgs,
			want:        true,
		},
		{
			name:        "rejects system:masters org",
			clusterName: cluster,
			signerName:  certificatesv1.KubeAPIServerClientSignerName,
			cn:          validCN,
			orgs:        []string{"system:masters"},
			want:        false,
		},
		{
			name:        "rejects wrong signer",
			clusterName: cluster,
			signerName:  "example.com/evil",
			cn:          validCN,
			orgs:        validOrgs,
			want:        false,
		},
		{
			name:        "rejects CN cluster mismatch",
			clusterName: cluster,
			signerName:  certificatesv1.KubeAPIServerClientSignerName,
			cn:          SubjectPrefix + "othercluster:" + agent,
			orgs:        validOrgs,
			want:        false,
		},
		{
			name:        "rejects malformed CN",
			clusterName: cluster,
			signerName:  certificatesv1.KubeAPIServerClientSignerName,
			cn:          "evil-user",
			orgs:        validOrgs,
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csr := &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-csr",
					Labels: map[string]string{
						constants.CSRClusterNameLabel: tt.clusterName,
					},
				},
				Spec: certificatesv1.CertificateSigningRequestSpec{
					SignerName: tt.signerName,
					Request:    newTestCSRRequest(t, tt.cn, tt.orgs),
				},
			}
			if got := ValidateClusterCSRRequest(csr, tt.clusterName); got != tt.want {
				t.Errorf("ValidateClusterCSRRequest() = %v, want %v", got, tt.want)
			}
		})
	}
}

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
