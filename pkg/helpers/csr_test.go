package helpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"testing"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			csr := &certificatesv1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{Name: "test-csr"},
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
