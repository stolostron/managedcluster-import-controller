package autoimport

import (
	"context"
	"testing"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func Test_updateClusterURL(t *testing.T) {
	cases := []struct {
		name            string
		secret          *corev1.Secret
		cluster         *clusterv1.ManagedCluster
		expectedErr     bool
		expectedCluster *clusterv1.ManagedCluster
	}{
		{
			name: "update cluster url kubeconfig",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"kubeconfig": newKubeConfig("https://ut.test.com:6443"),
				},
			},
			cluster:         newCluster("https://test.com"),
			expectedErr:     false,
			expectedCluster: newCluster("https://test.com", "https://ut.test.com:6443"),
		},
		{
			name: "update cluster url auto-import kubeconfig",
			secret: &corev1.Secret{
				Type: constants.AutoImportSecretKubeConfig,
				Data: map[string][]byte{
					"kubeconfig": newKubeConfig("https://ut.test.com:6443"),
				},
			},
			cluster:         newCluster("https://ut.test.com:6443"),
			expectedErr:     false,
			expectedCluster: newCluster("https://ut.test.com:6443"),
		},
		{
			name: "update cluster url token",
			secret: &corev1.Secret{
				Type: corev1.SecretTypeOpaque,
				Data: map[string][]byte{
					"server": []byte("https://ut.test.com:6443/"),
					"token":  []byte("token"),
				},
			},
			cluster:         newCluster(""),
			expectedErr:     false,
			expectedCluster: newCluster("https://ut.test.com:6443"),
		},
		{
			name: "update cluster url auto-import token",
			secret: &corev1.Secret{
				Type: constants.AutoImportSecretKubeToken,
				Data: map[string][]byte{
					"server": []byte("https://ut.test.com:6443/"),
					"token":  []byte("token"),
				},
			},
			cluster:         newCluster("https://test.com"),
			expectedErr:     false,
			expectedCluster: newCluster("https://ut.test.com:6443", "https://test.com"),
		},
		{
			name: "no cluster url",
			secret: &corev1.Secret{
				Type: constants.AutoImportSecretRosaConfig,
				Data: map[string][]byte{
					"server": []byte("https://ut.test.com:6443/"),
					"token":  []byte("token"),
				},
			},
			cluster:         newCluster("https://test.com"),
			expectedErr:     false,
			expectedCluster: newCluster("https://test.com"),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(testscheme).WithObjects(c.cluster).
				WithStatusSubresource(c.cluster).Build()
			err := updateClusterURL(context.Background(), client, c.cluster, c.secret)
			if err != nil && !c.expectedErr {
				t.Errorf("expected no error, but got %v", err)
			}
			if err == nil && c.expectedErr {
				t.Errorf("expected error, but got none")
			}
			cluster := &clusterv1.ManagedCluster{}
			err = client.Get(context.TODO(), types.NamespacedName{Name: c.cluster.Name}, cluster)
			if err != nil {
				t.Errorf("expected no error, but got %v", err)
			}

			if len(c.expectedCluster.Spec.ManagedClusterClientConfigs) != len(cluster.Spec.ManagedClusterClientConfigs) {
				t.Errorf("expected clusterConfig count %v, but got %v",
					len(c.expectedCluster.Spec.ManagedClusterClientConfigs), len(cluster.Spec.ManagedClusterClientConfigs))
			}

			urlUpdated := false
			for _, i := range c.expectedCluster.Spec.ManagedClusterClientConfigs {
				for _, j := range cluster.Spec.ManagedClusterClientConfigs {
					if i.URL == j.URL {
						urlUpdated = true
						break
					}
				}
			}
			if !urlUpdated {
				t.Errorf("expected updated cluster url, but not")
			}
		})
	}
}

func newKubeConfig(url string) []byte {
	kubeConfig := clientcmdapi.Config{
		Clusters: map[string]*clientcmdapi.Cluster{
			"test": &clientcmdapi.Cluster{
				Server:                url,
				InsecureSkipTLSVerify: false,
			},
		},
		CurrentContext: "test",
		Contexts: map[string]*clientcmdapi.Context{
			"test": &clientcmdapi.Context{
				Cluster: "test",
			},
		},
	}

	data, err := clientcmd.Write(kubeConfig)
	if err != nil {
		panic(err)
	}
	return data
}

func newCluster(urls ...string) *clusterv1.ManagedCluster {
	clusterClientConfigs := []clusterv1.ClientConfig{}
	for _, url := range urls {
		if url == "" {
			continue
		}
		clusterClientConfigs = append(clusterClientConfigs, clusterv1.ClientConfig{
			URL: url,
		})
	}
	return &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
		},
		Spec: clusterv1.ManagedClusterSpec{
			ManagedClusterClientConfigs: clusterClientConfigs,
		},
	}
}
