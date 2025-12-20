package helpers

import (
	"crypto/x509/pkix"
	"net"
	"testing"
	"time"

	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	certutil "k8s.io/client-go/util/cert"
	"k8s.io/client-go/util/keyutil"
)

func newCSR(commonName string, clusterName string, orgs ...string) *certificatesv1.CertificateSigningRequest {
	clientKey, _ := keyutil.MakeEllipticPrivateKeyPEM()
	privateKey, _ := keyutil.ParsePrivateKeyPEM(clientKey)

	request, _ := certutil.MakeCSR(privateKey, &pkix.Name{CommonName: commonName, Organization: orgs}, []string{"test.localhost"}, nil)

	return &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageClientAuth,
			},
			Username: "system:open-cluster-management:" + clusterName,
			Request:  request,
		},
	}
}

func TestCSRSignerWithExpiry(t *testing.T) {
	ca, key, err := certutil.GenerateSelfSignedCertKey("test", []net.IP{}, []string{})
	if err != nil {
		t.Errorf("Failed to generate self signed CA config: %v", err)
	}

	signer := CSRSignerWithExpiry(key, ca, 24*time.Hour)

	cert := signer(newCSR("test", "cluster1"))
	if cert == nil {
		t.Errorf("Expect cert to be signed")
	}

	certs, err := certutil.ParseCertsPEM(cert)
	if err != nil {
		t.Errorf("Failed to parse cert: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expect 1 cert signed but got %d", len(certs))
	}

	if certs[0].Subject.CommonName != "test" {
		t.Errorf("CommonName is not correct")
	}
}
