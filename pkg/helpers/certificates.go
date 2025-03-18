// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package helpers

import (
	"bytes"

	certutil "k8s.io/client-go/util/cert"
)

// HasCertificates checks if all certificates in subsetCertData are present in supersetCertData.
// Returns true if all certificates in subsetCertData are found in supersetCertData, false otherwise.
// Returns error if there is an issue parsing the certificates.
func HasCertificates(supersetCertData, subsetCertData []byte) (bool, error) {
	if len(subsetCertData) == 0 {
		return true, nil
	}

	supersetCerts, err := certutil.ParseCertsPEM(supersetCertData)
	if err != nil {
		return false, err
	}

	subsetCerts, err := certutil.ParseCertsPEM(subsetCertData)
	if err != nil {
		return false, err
	}

	for _, subsetCert := range subsetCerts {
		found := false
		for _, supersetCert := range supersetCerts {
			if bytes.Equal(subsetCert.Raw, supersetCert.Raw) {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	return true, nil
}
