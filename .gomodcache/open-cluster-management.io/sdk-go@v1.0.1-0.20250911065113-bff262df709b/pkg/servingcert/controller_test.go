package servingcert

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakekube "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/util/cert"
	"open-cluster-management.io/sdk-go/pkg/basecontroller/factory"
	"testing"
	"time"
)

func TestServingCertController_Sync(t *testing.T) {
	cases := []struct {
		name            string
		namespace       string
		existingObjects []runtime.Object
	}{
		{
			name:            "default no existing objects",
			namespace:       "test1",
			existingObjects: []runtime.Object{},
		},
		{
			name:      "have existing objects",
			namespace: "test1",
			existingObjects: []runtime.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test1", Name: "service-test1"},
				},
				&corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Namespace: "test1", Name: DefaultCABundleConfigmapName},
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kubeClient := fakekube.NewSimpleClientset(c.existingObjects...)
			ctx := context.TODO()

			controller := NewServingCertController(c.namespace, kubeClient).
				WithTargetServingCerts([]TargetServingCertOptions{
					{Name: "service-test1", HostNames: []string{"service-test1.svc"}},
					{Name: "service-test2", HostNames: []string{"service-test2.svc"}},
				})
			controller.Start(ctx)

			err := controller.sync(ctx, factory.NewSyncContext("test"), "test")
			if err != nil {
				t.Errorf("sync() error = %v", err)
			}

			assertResourcesExistAndValid(t, controller)
		})
	}
}

func assertResourcesExistAndValid(t *testing.T, controller *ServingCertController) {
	caBundleConfigMap, err := controller.kubeClient.CoreV1().ConfigMaps(controller.namespace).Get(context.Background(),
		DefaultCABundleConfigmapName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	caBundle := caBundleConfigMap.Data["ca-bundle.crt"]

	for _, target := range controller.targetRotations {
		secret, err := controller.kubeClient.CoreV1().Secrets(target.Namespace).Get(context.Background(), target.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			t.Fatalf("secret not found: %v", target.Name)
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		certificates, err := cert.ParseCertsPEM(secret.Data["tls.crt"])
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(certificates) == 0 {
			t.Fatalf("no certificate found")
		}

		now := time.Now()
		certificate := certificates[0]
		if now.After(certificate.NotAfter) {
			t.Fatalf("invalid NotAfter: %s", target.Name)
		}
		if now.Before(certificate.NotBefore) {
			t.Fatalf("invalid NotBefore: %s", target.Name)
		}

		if target.Name == DefaultSignerSecretName {
			continue
		}

		// ensure signing cert of serving certs in the ca bundle configmap
		caCerts, err := cert.ParseCertsPEM([]byte(caBundle))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		found := false
		for _, caCert := range caCerts {
			if certificate.Issuer.CommonName != caCert.Subject.CommonName {
				continue
			}
			if now.After(caCert.NotAfter) {
				t.Fatalf("invalid NotAfter of ca: %s", target.Name)
			}
			if now.Before(caCert.NotBefore) {
				t.Fatalf("invalid NotBefore of ca: %s", target.Name)
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("no issuer found: %s", target.Name)
		}
	}
}
