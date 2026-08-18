package helpers

import (
	"crypto/x509"
	"encoding/pem"
	"strings"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

const (
	// SubjectPrefix and ManagedClustersGroup match open-cluster-management registration agent identity.
	SubjectPrefix        = "system:open-cluster-management:"
	ManagedClustersGroup = SubjectPrefix + "managed-clusters"

	// GRPCAuthSigner is the signer name used when creating CSRs for gRPC authentication.
	GRPCAuthSigner = "open-cluster-management.io/grpc"
)

// IsAllowedAutoApproveSigner returns true for signers this controller may auto-approve.
func IsAllowedAutoApproveSigner(signerName string) bool {
	return signerName == certificatesv1.KubeAPIServerClientSignerName ||
		signerName == GRPCAuthSigner
}

// ValidateClusterCSRRequest validates signerName and the PEM-encoded x509 CSR Subject
// against the OCM registration agent identity contract (same rules as registration
// validateCSR). clusterName is the value from the open-cluster-management.io/cluster-name label.
//
// Expected:
//   - signerName: kubernetes.io/kube-apiserver-client or open-cluster-management.io/grpc
//   - CN: system:open-cluster-management:<cluster>:<agent>
//   - O: system:open-cluster-management:<cluster>
//     (system:open-cluster-management:managed-clusters is optional)
func ValidateClusterCSRRequest(csr *certificatesv1.CertificateSigningRequest, clusterName string) bool {
	if csr == nil || clusterName == "" {
		return false
	}
	if !IsAllowedAutoApproveSigner(csr.Spec.SignerName) {
		return false
	}

	block, _ := pem.Decode(csr.Spec.Request)
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return false
	}

	x509cr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		return false
	}

	requestingOrgs := sets.New(x509cr.Subject.Organization...)
	if requestingOrgs.Has(ManagedClustersGroup) {
		requestingOrgs.Delete(ManagedClustersGroup)
	}
	if requestingOrgs.Len() != 1 {
		return false
	}

	expectedPerClusterOrg := SubjectPrefix + clusterName
	if !requestingOrgs.Has(expectedPerClusterOrg) {
		return false
	}

	if !strings.HasPrefix(x509cr.Subject.CommonName, SubjectPrefix) {
		return false
	}

	nameWithoutPrefix := strings.TrimPrefix(x509cr.Subject.CommonName, SubjectPrefix)
	parts := strings.Split(nameWithoutPrefix, ":")
	if len(parts) != 2 {
		return false
	}

	clusterNameFromCN, agentName := parts[0], parts[1]
	if clusterNameFromCN == "" || agentName == "" {
		return false
	}
	if clusterNameFromCN != clusterName {
		return false
	}

	return true
}
