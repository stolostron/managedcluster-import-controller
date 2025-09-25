package helpers

import (
	"fmt"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	certificatesv1 "k8s.io/api/certificates/v1"
)

var BootstrapSASuffix = "bootstrap-sa"

const (
	GRPCSAName   = "grpc-server-sa"
	HubNamespace = "open-cluster-management-hub"
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
