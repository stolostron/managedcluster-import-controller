package cluster

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	jsonpatch "github.com/evanphx/json-patch/v5"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/statushash"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
)

func TestCreate(t *testing.T) {
	cases := []struct {
		name        string
		clusters    []runtime.Object
		newCluster  *clusterv1.ManagedCluster
		expectedErr string
	}{
		{
			name:     "new cluster",
			clusters: []runtime.Object{},
			newCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			expectedErr: "",
		},
		{
			name:     "existing cluster",
			clusters: []runtime.Object{&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}},
			newCluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
				},
			},
			expectedErr: "managedclusters.cluster.open-cluster-management.io \"cluster1\" already exists",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			watcherStore := store.NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
			ceClientOpt := fake.NewAgentOptions(gochan.New(), nil, "cluster1", "cluster1-agent")
			ceClient, err := generic.NewCloudEventAgentClient(
				ctx,
				ceClientOpt,
				store.NewAgentWatcherStoreLister(watcherStore),
				statushash.StatusHash,
				NewManagedClusterCodec())
			if err != nil {
				t.Error(err)
			}
			clusterClientSet := &ClusterClientSetWrapper{ClusterV1ClientWrapper: &ClusterV1ClientWrapper{
				ManagedClusterClient: NewManagedClusterClient(ceClient, watcherStore, "cluster1"),
			}}
			clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClientSet, time.Minute*10)
			clusterInformer := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer()
			clusterInformerStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()
			for _, cluster := range c.clusters {
				if err := clusterInformerStore.Add(cluster); err != nil {
					t.Error(err)
				}
			}
			watcherStore.SetInformer(clusterInformer)
			go clusterInformerFactory.Start(ctx.Done())

			if _, err = clusterClientSet.ClusterV1().ManagedClusters().Create(ctx, c.newCluster, metav1.CreateOptions{}); err != nil {
				if c.expectedErr == err.Error() {
					return
				}

				t.Error(err)
			}
		})
	}
}

func TestPatch(t *testing.T) {
	cases := []struct {
		name                string
		cluster             *clusterv1.ManagedCluster
		expectedKubeVersion string
	}{
		{
			name: "Patch a cluster",
			cluster: &clusterv1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					ResourceVersion: "0",
					Name:            "cluster1",
				},
			},
			expectedKubeVersion: "1.32",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			watcherStore := store.NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
			ceClientOpt := fake.NewAgentOptions(gochan.New(), nil, "cluster1", "test-agent")
			ceClient, err := generic.NewCloudEventAgentClient(
				ctx,
				ceClientOpt,
				store.NewAgentWatcherStoreLister(watcherStore),
				statushash.StatusHash,
				NewManagedClusterCodec())
			if err != nil {
				t.Error(err)
			}
			clusterClientSet := &ClusterClientSetWrapper{ClusterV1ClientWrapper: &ClusterV1ClientWrapper{
				ManagedClusterClient: NewManagedClusterClient(ceClient, watcherStore, "cluster1"),
			}}
			clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClientSet, time.Minute*10)
			clusterInformer := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer()
			clusterInformerStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()
			if err := clusterInformerStore.Add(c.cluster); err != nil {
				t.Error(err)
			}
			watcherStore.SetInformer(clusterInformer)
			go clusterInformerFactory.Start(ctx.Done())

			oldData, err := json.Marshal(c.cluster)
			if err != nil {
				t.Error(err)
			}

			newCluster := c.cluster.DeepCopy()
			newCluster.Status = clusterv1.ManagedClusterStatus{
				Version: clusterv1.ManagedClusterVersion{
					Kubernetes: c.expectedKubeVersion,
				},
			}

			newData, err := json.Marshal(newCluster)
			if err != nil {
				t.Error(err)
			}

			patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
			if err != nil {
				t.Error(err)
			}

			_, err = clusterClientSet.ClusterV1().ManagedClusters().Patch(
				ctx,
				c.cluster.Name,
				types.MergePatchType,
				patchBytes,
				metav1.PatchOptions{},
				"status",
			)
			if err != nil {
				t.Error(err)
			}
		})
	}
}
