package store

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	clusterfake "open-cluster-management.io/api/client/cluster/clientset/versioned/fake"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
)

func TestGet(t *testing.T) {
	clusterClient := clusterfake.NewSimpleClientset()
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, time.Minute*10)
	clusterStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()

	watchStore := NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
	watchStore.SetInformer(clusterInformerFactory.Cluster().V1().ManagedClusters().Informer())

	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "test1"}}); err != nil {
		t.Error(err)
	}
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "test2"}}); err != nil {
		t.Error(err)
	}

	cases := []struct {
		name        string
		clusterName string
		expected    bool
	}{
		{
			name:        "test1 exists",
			clusterName: "test1",
			expected:    true,
		},
		{
			name:        "test2 exists",
			clusterName: "test2",
			expected:    true,
		},
		{
			name:        "test does not exist",
			clusterName: "test",
			expected:    false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, exists, err := watchStore.Get("", c.clusterName)
			if err != nil {
				t.Error(err)
			}
			if exists != c.expected {
				t.Error("expect test1 exists, but failed")
			}
		})
	}
}

func TestList(t *testing.T) {
	clusterClient := clusterfake.NewSimpleClientset()
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, time.Minute*10)
	clusterStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "test1",
		Labels: map[string]string{"test": "true"},
	}}); err != nil {
		t.Error(err)
	}
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name:   "test2",
		Labels: map[string]string{"test": "true"},
	}}); err != nil {
		t.Error(err)
	}
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{
		Name: "test3",
	}}); err != nil {
		t.Error(err)
	}

	watchStore := NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
	watchStore.SetInformer(clusterInformerFactory.Cluster().V1().ManagedClusters().Informer())

	clusters, err := watchStore.List("", metav1.ListOptions{LabelSelector: "test=true"})
	if err != nil {
		t.Error(err)
	}

	if len(clusters.Items) != 2 {
		t.Error("expect 2, but failed")
	}

	all, err := watchStore.ListAll()
	if err != nil {
		t.Error(err)
	}

	if len(all) != 3 {
		t.Error("expect 2, but failed")
	}
}

type receiveResult struct {
	sync.RWMutex

	addedReceived    bool
	modifiedReceived bool
	deletedReceived  bool
}

func (r *receiveResult) added() {
	r.Lock()
	defer r.Unlock()
	r.addedReceived = true
}

func (r *receiveResult) updated() {
	r.Lock()
	defer r.Unlock()
	r.modifiedReceived = true
}

func (r *receiveResult) deleted() {
	r.Lock()
	defer r.Unlock()
	r.deletedReceived = true
}

func (r *receiveResult) result() bool {
	r.RLock()
	defer r.RUnlock()
	return r.addedReceived && r.modifiedReceived && r.deletedReceived
}

func TestWatch(t *testing.T) {
	clusterClient := clusterfake.NewSimpleClientset()
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, time.Minute*10)
	clusterStore := clusterInformerFactory.Cluster().V1().ManagedClusters().Informer().GetStore()
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "test1"}}); err != nil {
		t.Error(err)
	}
	if err := clusterStore.Add(&clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "test2"}}); err != nil {
		t.Error(err)
	}

	watchStore := NewAgentInformerWatcherStore[*clusterv1.ManagedCluster]()
	watchStore.SetInformer(clusterInformerFactory.Cluster().V1().ManagedClusters().Informer())
	watcher, err := watchStore.GetWatcher("", metav1.ListOptions{})
	if err != nil {
		t.Error(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		watcher.Stop()
		cancel()
	}()

	received := &receiveResult{}

	go func() {
		ch := watcher.ResultChan()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				switch event.Type {
				case watch.Added:
					received.added()
				case watch.Modified:
					received.updated()
				case watch.Deleted:
					received.deleted()
				}
			}
		}
	}()

	if err := watchStore.HandleReceivedResource(types.Added, &clusterv1.ManagedCluster{ObjectMeta: metav1.ObjectMeta{Name: "test0"}}); err != nil {
		t.Error(err)
	}
	if err := watchStore.HandleReceivedResource(types.Modified, &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test1",
		},
		Status: clusterv1.ManagedClusterStatus{
			Version: clusterv1.ManagedClusterVersion{Kubernetes: "1.23"},
		}}); err != nil {
		t.Error(err)
	}
	if err := watchStore.HandleReceivedResource(types.Deleted, &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test1",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
	}); err != nil {
		t.Error(err)
	}

	require.Eventually(t, func() bool {
		return received.result()
	}, 5*time.Second, time.Second)
}
