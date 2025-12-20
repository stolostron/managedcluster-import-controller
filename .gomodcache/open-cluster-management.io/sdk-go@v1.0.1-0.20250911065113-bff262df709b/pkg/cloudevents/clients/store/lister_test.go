package store

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	workfake "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/common"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

func TestAgentLister(t *testing.T) {
	clusterClient := clusterfake.NewSimpleClientset()
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, time.Minute*10)
	clusterStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()

	watchStore := NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
	watchStore.SetInformer(clusterInformerFactory.Cluster().V1().ManagedClusters().Informer())

	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "test1",
		Labels: map[string]string{common.CloudEventsOriginalSourceLabelKey: "source1"},
	}}); err != nil {
		t.Error(err)
	}
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "test2",
		Labels: map[string]string{common.CloudEventsOriginalSourceLabelKey: "source1"},
	}}); err != nil {
		t.Error(err)
	}

	agentLister := NewAgentWatcherStoreLister(watchStore)
	clusters, err := agentLister.List(types.ListOptions{Source: "source1"})
	if err != nil {
		t.Error(err)
	}
	if len(clusters) != 2 {
		t.Error("unexpected clusters")
	}
}

func TestSourceLister(t *testing.T) {
	workClient := workfake.NewSimpleClientset()
	workInformerFactory := workinformers.NewSharedInformerFactory(workClient, time.Minute*10)
	workStore := workInformerFactory.Work().V1().ManifestWorks().Informer().GetStore()

	watchStore := NewAgentInformerWatcherStore[*workv1.ManifestWork]()
	watchStore.SetInformer(workInformerFactory.Work().V1().ManifestWorks().Informer())

	if err := workStore.Add(&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
		Name:      "test1",
		Namespace: "cluster1",
	}}); err != nil {
		t.Error(err)
	}
	if err := workStore.Add(&workv1.ManifestWork{ObjectMeta: metav1.ObjectMeta{
		Name:      "test2",
		Namespace: "cluster2",
	}}); err != nil {
		t.Error(err)
	}

	agentLister := NewSourceWatcherStoreLister(watchStore)
	works, err := agentLister.List(types.ListOptions{ClusterName: "cluster1"})
	if err != nil {
		t.Error(err)
	}
	if len(works) != 1 {
		t.Error("unexpected works")
	}
}
