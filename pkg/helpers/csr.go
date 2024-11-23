package helpers

import (
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	certificatesv1 "k8s.io/api/certificates/v1"
)

func GetClusterName(csr *certificatesv1.CertificateSigningRequest) (clusterName string) {
	for label, v := range csr.GetObjectMeta().GetLabels() {
		if label == constants.CSRClusterNameLabel {
			clusterName = v
		}
	}
	return clusterName
}
