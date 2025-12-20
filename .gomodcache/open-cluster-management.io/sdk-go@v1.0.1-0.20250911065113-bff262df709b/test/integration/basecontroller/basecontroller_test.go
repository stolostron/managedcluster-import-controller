package basecontroller

import (
	"errors"
	"fmt"
	"github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

var _ = ginkgo.Describe("baseController test", func() {
	var err error
	var namespace string
	var configmapName string

	ginkgo.BeforeEach(func() {
		suffix := rand.String(5)
		namespace = fmt.Sprintf("test-%s", suffix)
		configmapName = fmt.Sprintf("configmap-%s", suffix)

		_, err = kubeClient.CoreV1().Namespaces().
			Create(ctx, &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}, metav1.CreateOptions{})
		gomega.Expect(err).ToNot(gomega.HaveOccurred())
	})

	ginkgo.Context("test controller", func() {
		ginkgo.It("sync should work", func() {
			configmap := &v1.ConfigMap{
				TypeMeta: metav1.TypeMeta{},
				ObjectMeta: metav1.ObjectMeta{
					Name:      configmapName,
					Namespace: namespace,
					Labels:    map[string]string{labelKey: "test1"},
				},
			}
			_, err = kubeClient.CoreV1().ConfigMaps(namespace).Create(ctx, configmap, metav1.CreateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			cm := &v1.ConfigMap{}
			gomega.Eventually(func() error {
				cm, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configmapName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if cm.Data == nil {
					return errors.New("configmap data is empty")
				}
				if cm.Data[labelKey] == "test1" {
					return nil
				}
				return errors.New("configmap data is not equal")
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())

			cm.SetLabels(map[string]string{labelKey: "test2"})
			_, err = kubeClient.CoreV1().ConfigMaps(namespace).Update(ctx, cm, metav1.UpdateOptions{})
			gomega.Expect(err).ToNot(gomega.HaveOccurred())

			gomega.Eventually(func() error {
				cm, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configmapName, metav1.GetOptions{})
				if err != nil {
					return err
				}
				if cm.Data == nil {
					return errors.New("configmap data is empty")
				}
				if cm.Data[labelKey] == "test2" {
					return nil
				}
				return errors.New("configmap data is not equal")
			}, eventuallyTimeout, eventuallyInterval).ShouldNot(gomega.HaveOccurred())
		})
	})
})
