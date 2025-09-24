package helpers

import (
	"testing"
	"time"

	apiconstants "github.com/stolostron/cluster-lifecycle-api/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func TestImportControllerConfig(t *testing.T) {
	cases := []struct {
		name                   string
		controllerConfig       *corev1.ConfigMap
		expectedStrategy       string
		expectedGenerateSecret bool
	}{
		{
			name:                   "default auto-import-strategy",
			expectedStrategy:       "ImportOnly",
			expectedGenerateSecret: false,
		},
		{
			name: "configmap without strategy config",
			controllerConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "import-controller-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"test": "test",
				},
			},
			expectedStrategy:       apiconstants.AutoImportStrategyImportOnly,
			expectedGenerateSecret: false,
		},
		{
			name: "configmap with invalid strategy config",
			controllerConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "import-controller-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"autoImportStrategy": "invalid-strategy",
				},
			},
			expectedStrategy:       apiconstants.AutoImportStrategyImportOnly,
			expectedGenerateSecret: false,
		},
		{
			name: "configmap with ImportAndSync strategy",
			controllerConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "import-controller-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"autoImportStrategy": apiconstants.AutoImportStrategyImportAndSync,
				},
			},
			expectedStrategy:       apiconstants.AutoImportStrategyImportAndSync,
			expectedGenerateSecret: false,
		},
		{
			name: "configmap with ImportOnly strategy",
			controllerConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "import-controller-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"autoImportStrategy": apiconstants.AutoImportStrategyImportOnly,
				},
			},
			expectedStrategy:       apiconstants.AutoImportStrategyImportOnly,
			expectedGenerateSecret: false,
		},
		{
			name: "configmap to disable generateSecret",
			controllerConfig: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "import-controller-config",
					Namespace: "test",
				},
				Data: map[string]string{
					"clusterImportConfig": "true",
				},
			},
			expectedStrategy:       apiconstants.AutoImportStrategyImportOnly,
			expectedGenerateSecret: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			objects := []runtime.Object{}
			if c.controllerConfig != nil {
				objects = append(objects, c.controllerConfig)
			}
			kubeClient := kubefake.NewSimpleClientset(objects...)
			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			configmapInformer := kubeInformerFactory.Core().V1().ConfigMaps().Informer()
			if c.controllerConfig != nil {
				configmapInformer.GetStore().Add(c.controllerConfig)
			}
			controllerConfig := NewImportControllerConfig("test",
				kubeInformerFactory.Core().V1().ConfigMaps().Lister(), logf.Log.WithName("import-controller-config"))
			autoImportStrategy, err := controllerConfig.GetAutoImportStrategy()
			if err != nil {
				t.Errorf("unexpected err %v", err)
			}
			generateSecret, err := controllerConfig.GenerateImportConfig()
			if err != nil {
				t.Errorf("unexpected err %v", err)
			}

			if c.expectedStrategy != autoImportStrategy {
				t.Errorf("expect %s, but got %s", c.expectedStrategy, autoImportStrategy)
			}
			if c.expectedGenerateSecret != generateSecret {
				t.Errorf("expect %v, but got %v", c.expectedGenerateSecret, generateSecret)
			}
		})
	}
}
