package servingcertcontroller

import (
	"bytes"
	"context"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/cert"
	"open-cluster-management.io/sdk-go/pkg/servingcert"
	"os"
	"time"
)

var _ = ginkgo.Describe("servingCert controller test", func() {
	ginkgo.AfterEach(func() {
		os.Remove("tls.crt")
		os.Remove("tls.key")

	})
	ginkgo.Context("test controller", func() {
		ginkgo.It("all servingCert secrets should be validated", func() {
			gomega.Eventually(func() error {
				err := validateCert()
				if err != nil {
					return err
				}
				return validateLoadSecret()
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

		})
		ginkgo.It("all servingCert secrets should be re-created after deleted", func() {
			err := kubeClient.CoreV1().Secrets(testNamespace).
				Delete(context.Background(), testTargetService1, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
			err = kubeClient.CoreV1().Secrets(testNamespace).
				Delete(context.Background(), testTargetService2, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				err := validateCert()
				if err != nil {
					return err
				}
				return validateLoadSecret()
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
		})
		ginkgo.It("all servingCert secrets should be updated after cabundle is deleted", func() {
			err := kubeClient.CoreV1().ConfigMaps(testNamespace).
				Delete(context.Background(), servingcert.DefaultCABundleConfigmapName, metav1.DeleteOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				return validateCert()

			}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
		})
	})
})

func validateCert() error {
	caBundleConfigMap, err := kubeClient.CoreV1().ConfigMaps(testNamespace).Get(context.Background(),
		servingcert.DefaultCABundleConfigmapName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get CA bundle configmap: %v", err)
	}

	caBundle := caBundleConfigMap.Data["ca-bundle.crt"]

	targetSecret := []string{testTargetService1, testTargetService2}
	for _, target := range targetSecret {
		secret, err := kubeClient.CoreV1().Secrets(testNamespace).Get(context.Background(), target, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return fmt.Errorf("secret not found: %v", target)
		}
		if err != nil {
			return fmt.Errorf("unexpected error to get secret %v: %v", target, err)
		}

		certificates, err := cert.ParseCertsPEM(secret.Data["tls.crt"])
		if err != nil {
			return fmt.Errorf("unexpected error to parse cert in the secret %v: %v", target, err)
		}
		if len(certificates) == 0 {
			return fmt.Errorf("no certificate found in the secret %v", target)
		}

		now := time.Now()
		certificate := certificates[0]
		if now.After(certificate.NotAfter) {
			return fmt.Errorf("invalid NotAfter in the cert of secret %s", target)
		}
		if now.Before(certificate.NotBefore) {
			return fmt.Errorf("invalid NotBefore in the cert of secret %s", target)
		}

		// ensure signing cert of serving certs in the ca bundle configmap
		caCerts, err := cert.ParseCertsPEM([]byte(caBundle))
		if err != nil {
			return fmt.Errorf("unexpected error to parse cert in CABundle: %v", err)
		}

		found := false
		for _, caCert := range caCerts {
			if certificate.Issuer.CommonName != caCert.Subject.CommonName {
				continue
			}
			if now.After(caCert.NotAfter) {
				return fmt.Errorf("invalid NotAfter of ca: %s", target)
			}
			if now.Before(caCert.NotBefore) {
				return fmt.Errorf("invalid NotBefore of ca: %s", target)
			}
			found = true
			break
		}
		if !found {
			return fmt.Errorf("no issuer found: %s", target)
		}
	}
	return nil
}

func validateLoadSecret() error {
	crt, err := os.ReadFile("tls.crt")
	if err != nil {
		return fmt.Errorf("failed to read tls.crt: %v", err)
	}
	key, err := os.ReadFile("tls.key")
	if err != nil {
		return fmt.Errorf("failed to read tls.key: %v", err)
	}

	targetSecret, err := kubeClient.CoreV1().Secrets(testNamespace).Get(context.Background(), testTargetService1, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get secret %v: %v", testTargetService1, err)
	}

	if !bytes.Equal([]byte(crt), targetSecret.Data["tls.crt"]) {
		return fmt.Errorf("invalid loaded tls.crt file in secret %v", testTargetService1)
	}
	if !bytes.Equal([]byte(key), targetSecret.Data["tls.key"]) {
		return fmt.Errorf("invalid loaded tls.key file in secret %v", testTargetService1)
	}
	return nil
}
