package v1beta2

import (
	"context"
	"os"
	"reflect"
	"testing"

	cliScheme "k8s.io/client-go/kubernetes/scheme"
	clusterv1beta2 "open-cluster-management.io/api/cluster/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/klog/v2"
	v1 "open-cluster-management.io/api/cluster/v1"
)

var (
	scheme = runtime.NewScheme()
)

type clustersGetter struct {
	client client.Client
}
type clusterSetsGetter struct {
	client client.Client
}
type clusterSetBindingsGetter struct {
	client client.Client
}

var existingClusterSetBindings = []*clusterv1beta2.ManagedClusterSetBinding{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "dev",
			Namespace: "default",
		},
		Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
			ClusterSet: "dev",
		},
		Status: clusterv1beta2.ManagedClusterSetBindingStatus{
			Conditions: []metav1.Condition{
				{
					Type:   clusterv1beta2.ClusterSetBindingBoundType,
					Status: metav1.ConditionTrue,
				},
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "global",
			Namespace: "default",
		},
		Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
			ClusterSet: "global",
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-such-cluster-set",
			Namespace: "kube-system",
		},
		Spec: clusterv1beta2.ManagedClusterSetBindingSpec{
			ClusterSet: "no-such-cluster-set",
		},
	},
}

var existingClusterSets = []*clusterv1beta2.ManagedClusterSet{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dev",
		},
		Spec: clusterv1beta2.ManagedClusterSetSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "global",
		},
		Spec: clusterv1beta2.ManagedClusterSetSpec{
			ClusterSelector: clusterv1beta2.ManagedClusterSelector{
				SelectorType:  clusterv1beta2.LabelSelector,
				LabelSelector: &metav1.LabelSelector{},
			},
		},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "openshift",
		},
		Spec: clusterv1beta2.ManagedClusterSetSpec{
			ClusterSelector: clusterv1beta2.ManagedClusterSelector{
				SelectorType: clusterv1beta2.LabelSelector,
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"vendor": "openshift",
					},
				},
			},
		},
	},
}

var existingClusters = []*v1.ManagedCluster{
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c1",
			Labels: map[string]string{
				"vendor":                       "openshift",
				clusterv1beta2.ClusterSetLabel: "dev",
			},
		},
		Spec: v1.ManagedClusterSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c2",
			Labels: map[string]string{
				"cloud":                        "aws",
				"vendor":                       "openshift",
				clusterv1beta2.ClusterSetLabel: "dev",
			},
		},
		Spec: v1.ManagedClusterSpec{},
	},
	{
		ObjectMeta: metav1.ObjectMeta{
			Name: "c3",
			Labels: map[string]string{
				"cloud": "aws",
			},
		},
		Spec: v1.ManagedClusterSpec{},
	},
}

func TestMain(m *testing.M) {
	if err := v1.AddToScheme(cliScheme.Scheme); err != nil {
		klog.Errorf("Failed adding cluster to scheme, %v", err)
		os.Exit(1)
	}
	if err := clusterv1beta2.Install(cliScheme.Scheme); err != nil {
		klog.Errorf("Failed adding set to scheme, %v", err)
		os.Exit(1)
	}

	if err := v1.Install(scheme); err != nil {
		klog.Errorf("Failed adding cluster to scheme, %v", err)
		os.Exit(1)
	}
	if err := clusterv1beta2.Install(scheme); err != nil {
		klog.Errorf("Failed adding set to scheme, %v", err)
		os.Exit(1)
	}

	exitVal := m.Run()
	os.Exit(exitVal)
}

func (mcl clustersGetter) List(selector labels.Selector) ([]*v1.ManagedCluster, error) {
	clusterList := v1.ManagedClusterList{}
	err := mcl.client.List(context.Background(), &clusterList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var retClusters []*v1.ManagedCluster
	for i := range clusterList.Items {
		retClusters = append(retClusters, &clusterList.Items[i])
	}
	return retClusters, nil
}

func (msl clusterSetsGetter) List(selector labels.Selector) ([]*clusterv1beta2.ManagedClusterSet, error) {
	clusterSetList := clusterv1beta2.ManagedClusterSetList{}
	err := msl.client.List(context.Background(), &clusterSetList, &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var retClusterSets []*clusterv1beta2.ManagedClusterSet
	for i := range clusterSetList.Items {
		retClusterSets = append(retClusterSets, &clusterSetList.Items[i])
	}
	return retClusterSets, nil
}

func (mbl clusterSetBindingsGetter) List(namespace string,
	selector labels.Selector) ([]*clusterv1beta2.ManagedClusterSetBinding, error) {
	clusterSetBindingList := clusterv1beta2.ManagedClusterSetBindingList{}
	err := mbl.client.List(context.Background(), &clusterSetBindingList,
		client.InNamespace(namespace), &client.ListOptions{LabelSelector: selector})
	if err != nil {
		return nil, err
	}
	var retClusterSetBindings []*clusterv1beta2.ManagedClusterSetBinding
	for i := range clusterSetBindingList.Items {
		retClusterSetBindings = append(retClusterSetBindings, &clusterSetBindingList.Items[i])
	}
	return retClusterSetBindings, nil
}

func TestGetClustersFromClusterSet(t *testing.T) {
	tests := []struct {
		name               string
		clusterset         *clusterv1beta2.ManagedClusterSet
		expectClustersName sets.Set[string]
		expectError        bool
	}{
		{
			name: "test default cluster set",
			clusterset: &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dev",
				},
				Spec: clusterv1beta2.ManagedClusterSetSpec{},
			},
			expectClustersName: sets.New[string]("c1", "c2"),
		},
		{
			name: "test exclusive cluster set",
			clusterset: &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "dev",
				},
				Spec: clusterv1beta2.ManagedClusterSetSpec{
					ClusterSelector: clusterv1beta2.ManagedClusterSelector{
						SelectorType: clusterv1beta2.ExclusiveClusterSetLabel,
					},
				},
			},
			expectClustersName: sets.New[string]("c1", "c2"),
		},
		{
			name: "test label selector(openshift) cluster set",
			clusterset: &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "openshift",
				},
				Spec: clusterv1beta2.ManagedClusterSetSpec{
					ClusterSelector: clusterv1beta2.ManagedClusterSelector{
						SelectorType: clusterv1beta2.LabelSelector,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"vendor": "openshift",
							},
						},
					},
				},
			},
			expectClustersName: sets.New[string]("c1", "c2"),
		},
		{
			name: "test global cluster set",
			clusterset: &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "global",
				},
				Spec: clusterv1beta2.ManagedClusterSetSpec{
					ClusterSelector: clusterv1beta2.ManagedClusterSelector{
						SelectorType:  clusterv1beta2.LabelSelector,
						LabelSelector: &metav1.LabelSelector{},
					},
				},
			},
			expectClustersName: sets.New[string]("c1", "c2", "c3"),
		},
		{
			name: "test label selector cluster set",
			clusterset: &clusterv1beta2.ManagedClusterSet{
				ObjectMeta: metav1.ObjectMeta{
					Name: "invalidset",
				},
				Spec: clusterv1beta2.ManagedClusterSetSpec{
					ClusterSelector: clusterv1beta2.ManagedClusterSelector{
						SelectorType: "invalidType",
					},
				},
			},
			expectError: true,
		},
	}

	var existingObjs []client.Object
	for _, cluster := range existingClusters {
		existingObjs = append(existingObjs, cluster)
	}
	mcl := clustersGetter{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build(),
	}

	for _, test := range tests {
		clusters, err := GetClustersFromClusterSet(test.clusterset, mcl)
		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run GetClustersFromClusterSet with clusterset: %v", test.name, test.clusterset)
			return
		}
		returnClusters := convertClusterToSet(clusters)
		if !reflect.DeepEqual(returnClusters, test.expectClustersName) {
			t.Errorf("Case: %v, Failed to run GetClustersFromClusterSet. Expect clusters: %v, return cluster: %v", test.name, test.expectClustersName, returnClusters)
			return
		}
	}
}

func TestGetClusterSetsOfCluster(t *testing.T) {
	tests := []struct {
		name                 string
		cluster              v1.ManagedCluster
		expectClusterSetName sets.Set[string]
		expectError          bool
	}{
		{
			name: "test c1 cluster",
			cluster: v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c1",
					Labels: map[string]string{
						"vendor":                       "openshift",
						clusterv1beta2.ClusterSetLabel: "dev",
					},
				},
				Spec: v1.ManagedClusterSpec{},
			},
			expectClusterSetName: sets.New[string]("dev", "openshift", "global"),
		},
		{
			name: "test c2 cluster",
			cluster: v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud":                        "aws",
						"vendor":                       "openshift",
						clusterv1beta2.ClusterSetLabel: "dev",
					},
				},
				Spec: v1.ManagedClusterSpec{},
			},
			expectClusterSetName: sets.New[string]("dev", "openshift", "global"),
		},
		{
			name: "test c3 cluster",
			cluster: v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "c2",
					Labels: map[string]string{
						"cloud": "aws",
					},
				},
				Spec: v1.ManagedClusterSpec{},
			},
			expectClusterSetName: sets.New[string]("global"),
		},
		{
			name: "test nonexist cluster in client",
			cluster: v1.ManagedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name: "doNotExistCluster",
					Labels: map[string]string{
						"cloud":  "aws",
						"vendor": "openshift",
					},
				},
				Spec: v1.ManagedClusterSpec{},
			},
			expectClusterSetName: sets.New[string]("openshift", "global"),
		},
	}

	var existingObjs []client.Object
	for _, cluster := range existingClusters {
		existingObjs = append(existingObjs, cluster)
	}
	for _, clusterset := range existingClusterSets {
		existingObjs = append(existingObjs, clusterset)
	}

	msl := clusterSetsGetter{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build(),
	}

	for _, test := range tests {
		returnSets, err := GetClusterSetsOfCluster(&test.cluster, msl)

		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run GetClusterSetsOfCluster with cluster: %v", test.name, test.cluster)
			return
		}
		returnClusters := convertClusterSetToSet(returnSets)
		if !reflect.DeepEqual(returnClusters, test.expectClusterSetName) {
			t.Errorf("Case: %v, Failed to run GetClusterSetsOfCluster. Expect clusters: %v, return cluster: %v", test.name, test.expectClusterSetName, returnClusters)
			return
		}
	}
}

func convertClusterToSet(clusters []*v1.ManagedCluster) sets.Set[string] {
	if len(clusters) == 0 {
		return nil
	}
	retSet := sets.New[string]()
	for _, cluster := range clusters {
		retSet.Insert(cluster.Name)
	}
	return retSet
}

func convertClusterSetToSet(clustersets []*clusterv1beta2.ManagedClusterSet) sets.Set[string] {
	if len(clustersets) == 0 {
		return nil
	}
	retSet := sets.New[string]()
	for _, clusterset := range clustersets {
		retSet.Insert(clusterset.Name)
	}
	return retSet
}

func convertClusterSetBindingsToSet(clusterSetBindings []*clusterv1beta2.ManagedClusterSetBinding) sets.Set[string] {
	if len(clusterSetBindings) == 0 {
		return nil
	}
	retSet := sets.New[string]()
	for _, clusterSetBinding := range clusterSetBindings {
		retSet.Insert(clusterSetBinding.Name)
	}
	return retSet
}

func TestGetValidManagedClusterSetBindings(t *testing.T) {
	tests := []struct {
		name                          string
		namespace                     string
		expectClusterSetBindingsNames sets.Set[string]
		expectError                   bool
	}{
		{
			name:                          "test found valid cluster bindings only",
			namespace:                     "default",
			expectClusterSetBindingsNames: sets.New[string]("dev"),
		},

		{
			name:                          "test no cluster binding found",
			namespace:                     "kube-system",
			expectClusterSetBindingsNames: nil,
		},
	}

	var existingObjs []client.Object
	for _, cluster := range existingClusters {
		existingObjs = append(existingObjs, cluster)
	}
	for _, clusterSet := range existingClusterSets {
		existingObjs = append(existingObjs, clusterSet)
	}
	for _, clusterSetBinding := range existingClusterSetBindings {
		existingObjs = append(existingObjs, clusterSetBinding)
	}

	mbl := clusterSetBindingsGetter{
		client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingObjs...).Build(),
	}

	for _, test := range tests {
		returnSets, err := GetBoundManagedClusterSetBindings(test.namespace, mbl)

		if err != nil {
			if test.expectError {
				continue
			}
			t.Errorf("Case: %v, Failed to run GetValidManagedClusterSetBindings with namespace: %v", test.name, test.namespace)
			return
		}
		returnBindings := convertClusterSetBindingsToSet(returnSets)
		if !reflect.DeepEqual(returnBindings, test.expectClusterSetBindingsNames) {
			t.Errorf("Case: %v, Failed to run GetValidManagedClusterSetBindings. Expect bindings: %v, return bindings: %v", test.name, test.expectClusterSetBindingsNames, returnBindings)
			return
		}
	}
}
