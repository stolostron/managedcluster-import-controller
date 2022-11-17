// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package e2e

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"

	"github.com/stolostron/managedcluster-import-controller/pkg/helpers/imageregistry"
	"github.com/stolostron/managedcluster-import-controller/test/e2e/util"
)

var _ = ginkgo.Describe("Using customized image registry", func() {
	var (
		managedClusterName string
		pullSecretName     string
	)

	ginkgo.BeforeEach(func() {
		managedClusterName = fmt.Sprintf("imageregistry-test-%s", rand.String(6))
		pullSecretName = fmt.Sprintf("pull-secret-%s", rand.String(6))

		ginkgo.By(fmt.Sprintf("Create pull secret %s", pullSecretName), func() {
			err := util.CreatePullSecret(hubKubeClient, "default", pullSecretName)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster namespace %s", managedClusterName), func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: managedClusterName}}
			_, err := hubKubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})

		ginkgo.By(fmt.Sprintf("Create managed cluster %s", managedClusterName), func() {
			imageRegistries := imageregistry.ImageRegistries{
				PullSecret: fmt.Sprintf("%s.%s", "default", pullSecretName),
				Registries: []imageregistry.Registry{
					{
						Source: "quay.io/open-cluster-management",
						Mirror: "quay.io/rhacm2",
					},
				},
			}
			_, err := util.CreateClusterWithImageRegistries(hubClusterClient, managedClusterName, imageRegistries)
			gomega.Expect(err).ToNot(gomega.HaveOccurred())
		})
	})

	ginkgo.AfterEach(func() {
		assertManagedClusterDeleted(managedClusterName)
		assertPullSecretDeleted("default", pullSecretName)
	})

	ginkgo.It("Should using customized image registry", func() {
		assertManagedClusterImportSecretCreated(managedClusterName, "other")

		ginkgo.By("Check image registry", func() {
			name := fmt.Sprintf("%s-import", managedClusterName)
			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(context.TODO(), name, metav1.GetOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			importYaml, ok := secret.Data["import.yaml"]
			gomega.Expect(ok).Should(gomega.BeTrue())

			objs := util.ToImportResoruces(importYaml)

			hasImagePullCredentials := false
			hasCustomizedImage := false
			for _, obj := range objs {
				if obj.GetName() == "open-cluster-management-image-pull-credentials" && obj.GetKind() == "Secret" {
					hasImagePullCredentials = true
				}

				if obj.GetName() == "klusterlet" && obj.GetKind() == "Klusterlet" {
					klusterlet := util.ToKlusterlet(obj)
					if klusterlet.Spec.WorkImagePullSpec == "quay.io/rhacm2/work:latest" &&
						klusterlet.Spec.RegistrationImagePullSpec == "quay.io/rhacm2/registration:latest" {
						hasCustomizedImage = true
					}
				}
			}

			gomega.Expect(hasImagePullCredentials).Should(gomega.BeTrue())
			gomega.Expect(hasCustomizedImage).Should(gomega.BeTrue())
		})
	})
})
