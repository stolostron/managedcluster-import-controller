package garbagecollector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/metadata"
	fakemetadata "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	workclientset "open-cluster-management.io/api/client/work/clientset/versioned"
	fakework "open-cluster-management.io/api/client/work/clientset/versioned/fake"
	workinformers "open-cluster-management.io/api/client/work/informers/externalversions"
	workapiv1 "open-cluster-management.io/api/work/v1"
)

func buildWork(namespace, name string, owners []string) *workapiv1.ManifestWork {
	var ownerReferences []metav1.OwnerReference
	for i := 0; i < len(owners); i++ {
		ownerReferences = append(ownerReferences, metav1.OwnerReference{UID: types.UID(owners[i])})
	}

	return &workapiv1.ManifestWork{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerReferences,
		},
	}
}

func TestProcessManifestWorkEvent(t *testing.T) {
	cases := []struct {
		name string
		// a series of events that will be supplied to the ownerChanges queue
		manifestworks []*workapiv1.ManifestWork
		expectedIndex map[string][]string
	}{
		{
			name: "test1",
			manifestworks: []*workapiv1.ManifestWork{
				buildWork("ns1", "work1", []string{}),
				buildWork("ns1", "work2", []string{"1"}),
				buildWork("ns1", "work3", []string{"1", "3"}),
				buildWork("ns1", "work1", []string{"1"}),
				buildWork("ns1", "work2", []string{"1", "2"}),
				buildWork("ns1", "work3", []string{"3"}),
			},
			expectedIndex: map[string][]string{
				"1": {"ns1/work1", "ns1/work2"},
				"2": {"ns1/work2"},
				"3": {"ns1/work3"},
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fakeWorkClient := fakework.NewSimpleClientset()
	fakeWorkInformer := workinformers.NewSharedInformerFactory(fakeWorkClient, 10*time.Minute).Work().V1().ManifestWorks()
	metadataClient := fakemetadata.NewSimpleMetadataClient(fakemetadata.NewTestScheme())
	if err := fakeWorkInformer.Informer().AddIndexers(cache.Indexers{
		manifestWorkByOwner: indexManifestWorkByOwner,
	}); err != nil {
		utilruntime.HandleError(fmt.Errorf("failed to add indexers: %v", err))
	}
	gc := &GarbageCollector{
		workClient:     fakeWorkClient.WorkV1(),
		workIndexer:    fakeWorkInformer.Informer().GetIndexer(),
		workInformer:   fakeWorkInformer,
		metadataClient: metadataClient,
		attemptToDelete: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*dependent](),
			workqueue.TypedRateLimitingQueueConfig[*dependent]{Name: "garbage_collector_attempt_to_delete"},
		),
	}
	go fakeWorkInformer.Informer().Run(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), fakeWorkInformer.Informer().HasSynced) {
		t.Fatalf("failed to sync informer cache")
	}

	for _, testCase := range cases {
		for _, work := range testCase.manifestworks {
			if err := gc.workInformer.Informer().GetStore().Add(work); err != nil {
				if !errors.IsAlreadyExists(err) {
					t.Fatalf("failed to add work to store: %v", err)
				}
				if err := gc.workInformer.Informer().GetStore().Update(work); err != nil {
					t.Fatalf("failed to update work in store: %v", err)
				}
			}
		}

		for ownerUID, expectedNamespacedNames := range testCase.expectedIndex {
			objs, err := gc.workIndexer.ByIndex(manifestWorkByOwner, ownerUID)
			if err != nil {
				t.Fatalf("failed to get objects by index: %v", err)
			}
			gotNamspacedNames := make([]string, len(objs))
			for i, o := range objs {
				manifestWork := o.(*workapiv1.ManifestWork)
				gotNamspacedNames[i] = types.NamespacedName{Name: manifestWork.Name, Namespace: manifestWork.Namespace}.String()
			}
			trans := cmp.Transformer("Sort", func(in []string) []string {
				sort.Strings(in)
				return in
			})
			if !cmp.Equal(expectedNamespacedNames, gotNamspacedNames, trans) {
				t.Fatalf("expected %v but got %v", expectedNamespacedNames, gotNamspacedNames)
			}
		}
	}
}

// fakeAction records information about requests to aid in testing.
type fakeAction struct {
	method string
	path   string
	query  string
}

// String returns method=path to aid in testing
func (f *fakeAction) String() string {
	return strings.Join([]string{f.method, f.path}, "=")
}

type FakeResponse struct {
	statusCode int
	content    []byte
}

// fakeActionHandler holds a list of fakeActions received
type fakeActionHandler struct {
	// statusCode and content returned by this handler for different method + path.
	response map[string]FakeResponse

	lock    sync.Mutex
	actions []fakeAction
}

// ServeHTTP logs the action that occurred and always returns the associated status code
func (f *fakeActionHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	func() {
		f.lock.Lock()
		defer f.lock.Unlock()

		f.actions = append(f.actions, fakeAction{method: request.Method, path: request.URL.Path, query: request.URL.RawQuery})
		fakeResponse, ok := f.response[request.Method+request.URL.Path]
		if !ok {
			fakeResponse.statusCode = 200
			fakeResponse.content = []byte(`{"apiVersion": "v1", "kind": "List"}`)
		}
		response.Header().Set("Content-Type", "application/json")
		response.WriteHeader(fakeResponse.statusCode)
		_, err := response.Write(fakeResponse.content)
		if err != nil {
			return
		}
	}()

	// This is to allow the fakeActionHandler to simulate a watch being opened
	if strings.Contains(request.URL.RawQuery, "watch=true") {
		hijacker, ok := response.(http.Hijacker)
		if !ok {
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		defer connection.Close()
		time.Sleep(30 * time.Second)
	}
}

// testServerAndClientConfig returns a server that listens and a config that can reference it
func testServerAndClientConfig(handler func(http.ResponseWriter, *http.Request)) (*httptest.Server, *rest.Config) {
	srv := httptest.NewServer(http.HandlerFunc(handler))
	config := &rest.Config{
		Host: srv.URL,
	}
	return srv, config
}

func setupGC(t *testing.T, config *rest.Config) *GarbageCollector {
	workClient, err := workclientset.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	metadataClient, err := metadata.NewForConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	workInformer := workinformers.NewSharedInformerFactory(workClient, 0)

	listOptions := &metav1.ListOptions{
		LabelSelector: "test=test",
		FieldSelector: "metadata.name=test",
	}
	ownerGVRFilters := map[schema.GroupVersionResource]*metav1.ListOptions{
		addonapiv1alpha1.SchemeGroupVersion.WithResource("managedclusteraddons"):    listOptions,
		addonapiv1alpha1.SchemeGroupVersion.WithResource("clustermanagementaddons"): listOptions,
	}

	return &GarbageCollector{
		workClient:      workClient.WorkV1(),
		workIndexer:     workInformer.Work().V1().ManifestWorks().Informer().GetIndexer(),
		workInformer:    workInformer.Work().V1().ManifestWorks(),
		metadataClient:  metadataClient,
		ownerGVRFilters: ownerGVRFilters,
		attemptToDelete: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[*dependent](),
			workqueue.TypedRateLimitingQueueConfig[*dependent]{Name: "garbage_collector_attempt_to_delete"},
		),
	}
}

func getWork(workName, workNamespace, workUID string, ownerReferences []metav1.OwnerReference) *workapiv1.ManifestWork {
	return &workapiv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ManifestWork",
			APIVersion: "work.open-cluster-management.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            workName,
			Namespace:       workNamespace,
			UID:             types.UID(workUID),
			OwnerReferences: ownerReferences,
		},
	}
}

func serilizeOrDie(t *testing.T, object interface{}) []byte {
	data, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestAttemptToDeleteItem(t *testing.T) {
	workName := "ToBeDeletedWork"
	workNamespace := "ns1"
	workUID := "123"
	ownerUID := "456"
	ownerName := "addon1"
	work := getWork(workName, workNamespace, workUID, []metav1.OwnerReference{
		{
			APIVersion: "addon.open-cluster-management.io/v1alpha1",
			Kind:       "ManagedClusterAddon",
			Name:       ownerName,
			UID:        types.UID(ownerUID),
		},
	})
	testHandler := &fakeActionHandler{
		response: map[string]FakeResponse{
			"GET" + "/apis/addon.open-cluster-management.io/v1alpha1/namespaces/ns1/managedclusteraddons/addon1": {
				404,
				[]byte{},
			},
			"GET" + "/apis/work.open-cluster-management.io/v1/namespaces/ns1/manifestworks/ToBeDeletedWork": {
				200,
				serilizeOrDie(t, work),
			},
			"PATCH" + "/apis/work.open-cluster-management.io/v1/namespaces/ns1/manifestworks/ToBeDeletedWork": {
				200,
				serilizeOrDie(t, work),
			},
		},
	}
	srv, clientConfig := testServerAndClientConfig(testHandler.ServeHTTP)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gc := setupGC(t, clientConfig)
	item := &dependent{
		ownerUID: types.UID(ownerUID),
		namespacedName: types.NamespacedName{
			Name:      workName,
			Namespace: workNamespace,
		},
	}
	itemAction := gc.attemptToDeleteWorker(ctx, item)
	if itemAction != forgetItem {
		t.Errorf("attemptToDeleteWorker returned unexpected action: %v", itemAction)
	}
	expectedActionSet := sets.NewString()
	expectedActionSet.Insert("GET=/apis/work.open-cluster-management.io/v1/namespaces/ns1/manifestworks/ToBeDeletedWork")
	expectedActionSet.Insert("DELETE=/apis/work.open-cluster-management.io/v1/namespaces/ns1/manifestworks/ToBeDeletedWork")

	actualActionSet := sets.NewString()
	for _, action := range testHandler.actions {
		actualActionSet.Insert(action.String())
	}
	if !expectedActionSet.Equal(actualActionSet) {
		t.Errorf("expected actions:\n%v\n but got:\n%v\nDifference:\n%v", expectedActionSet,
			actualActionSet, expectedActionSet.Difference(actualActionSet))
	}
}
