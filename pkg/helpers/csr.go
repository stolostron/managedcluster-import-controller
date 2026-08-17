package helpers

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/util/sets"
)

var BootstrapSASuffix = "bootstrap-sa"

const (
	GRPCSAName   = "grpc-server-sa"
	HubNamespace = "open-cluster-management-hub"

	// SubjectPrefix and ManagedClustersGroup match open-cluster-management registration agent identity.
	SubjectPrefix        = "system:open-cluster-management:"
	ManagedClustersGroup = SubjectPrefix + "managed-clusters"

	// GRPCAuthSigner is the signer name used when creating CSRs for gRPC authentication.
	// Duplicated as a string so older z-streams without operator/v1.GRPCAuthSigner still compile.
	GRPCAuthSigner = "open-cluster-management.io/grpc"
)

func GetClusterName(csr *certificatesv1.CertificateSigningRequest) (clusterName string) {
	for label, v := range csr.GetObjectMeta().GetLabels() {
		if label == constants.CSRClusterNameLabel {
			clusterName = v
		}
	}
	return clusterName
}

func GetBootstrapSAName(clusterName string) string {
	bootstrapSAName := fmt.Sprintf("%s-%s", clusterName, BootstrapSASuffix)
	if len(bootstrapSAName) > 63 {
		return fmt.Sprintf("%s-%s", clusterName[:63-len("-"+BootstrapSASuffix)], BootstrapSASuffix)
	}
	return bootstrapSAName
}

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
