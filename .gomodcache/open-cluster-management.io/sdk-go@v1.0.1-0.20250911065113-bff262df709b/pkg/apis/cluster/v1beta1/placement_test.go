package v1beta1

import (
	"reflect"
	"strconv"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
)

type FakePlacementDecisionGetter struct {
	FakeDecisions []*clusterv1beta1.PlacementDecision
}

func (f *FakePlacementDecisionGetter) List(selector labels.Selector, namespace string) (ret []*clusterv1beta1.PlacementDecision, err error) {
	return f.FakeDecisions, nil
}

func (f *FakePlacementDecisionGetter) Update(newPlacementDecisions []*clusterv1beta1.PlacementDecision) (ret []*clusterv1beta1.PlacementDecision, err error) {
	f.FakeDecisions = newPlacementDecisions
	return f.FakeDecisions, nil
}

func newFakePlacementDecision(placementName, groupName string, groupIndex int, clusterNames ...string) *clusterv1beta1.PlacementDecision {
	decisions := make([]clusterv1beta1.ClusterDecision, len(clusterNames))
	for i, clusterName := range clusterNames {
		decisions[i] = clusterv1beta1.ClusterDecision{ClusterName: clusterName}
	}

	return &clusterv1beta1.PlacementDecision{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				clusterv1beta1.PlacementLabel:          placementName,
				clusterv1beta1.DecisionGroupNameLabel:  groupName,
				clusterv1beta1.DecisionGroupIndexLabel: strconv.Itoa(groupIndex),
			},
		},
		Status: clusterv1beta1.PlacementDecisionStatus{
			Decisions: decisions,
		},
	}
}

func TestPlacementDecisionClustersTracker_GetClusterChanges(t *testing.T) {
	tests := []struct {
		name                           string
		placement                      clusterv1beta1.Placement
		existingScheduledClusterGroups map[GroupKey]sets.Set[string]
		updateDecisions                []*clusterv1beta1.PlacementDecision
		expectAddedScheduledClusters   sets.Set[string]
		expectDeletedScheduledClusters sets.Set[string]
	}{
		{
			name: "test placementdecisions",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			existingScheduledClusterGroups: map[GroupKey]sets.Set[string]{
				{GroupName: "", GroupIndex: 0}: sets.New[string]("cluster1", "cluster2"),
			},
			updateDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "", 0, "cluster1"),
				newFakePlacementDecision("placement1", "", 0, "cluster3"),
			},
			expectAddedScheduledClusters:   sets.New[string]("cluster3"),
			expectDeletedScheduledClusters: sets.New[string]("cluster2"),
		},
		{
			name: "test empty placementdecision",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement2", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			existingScheduledClusterGroups: map[GroupKey]sets.Set[string]{},
			updateDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement2", "", 0, "cluster1", "cluster2"),
			},
			expectAddedScheduledClusters:   sets.New[string]("cluster1", "cluster2"),
			expectDeletedScheduledClusters: sets.New[string](),
		},
		{
			name: "test nil exist cluster groups",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement2", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			existingScheduledClusterGroups: nil,
			updateDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement2", "", 0, "cluster1", "cluster2"),
			},
			expectAddedScheduledClusters:   sets.New[string]("cluster1", "cluster2"),
			expectDeletedScheduledClusters: sets.New[string](),
		},
	}

	for _, test := range tests {
		// init fake placement decision getter
		fakeGetter := FakePlacementDecisionGetter{
			FakeDecisions: test.updateDecisions,
		}
		// init tracker
		tracker := NewPlacementDecisionClustersTrackerWithGroups(&test.placement, &fakeGetter, test.existingScheduledClusterGroups)

		// check changed decision clusters
		addedClusters, deletedClusters, err := tracker.GetClusterChanges()
		if err != nil {
			t.Errorf("Case: %v, Failed to run Get(): %v", test.name, err)
		}
		if !reflect.DeepEqual(addedClusters, test.expectAddedScheduledClusters) {
			t.Errorf("Case: %v, expect added decisions: %v, return decisions: %v", test.name, test.expectAddedScheduledClusters, addedClusters)
			return
		}
		if !reflect.DeepEqual(deletedClusters, test.expectDeletedScheduledClusters) {
			t.Errorf("Case: %v, expect deleted decisions: %v, return decisions: %v", test.name, test.expectDeletedScheduledClusters, deletedClusters)
			return
		}
	}
}

func TestPlacementDecisionClustersTracker_Existing(t *testing.T) {
	tests := []struct {
		name                            string
		placement                       clusterv1beta1.Placement
		placementDecisions              []*clusterv1beta1.PlacementDecision
		groupKeys                       []GroupKey
		expectedExistingClusters        sets.Set[string]
		expectedExistingBesidesClusters sets.Set[string]
	}{
		{
			name: "test full group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group2", 1, "cluster3", "cluster4"),
			},
			groupKeys: []GroupKey{
				{GroupName: "group1"},
				{GroupIndex: 1},
				{GroupName: "group3"},
			},
			expectedExistingClusters:        sets.New[string]("cluster1", "cluster2", "cluster3", "cluster4"),
			expectedExistingBesidesClusters: sets.New[string](),
		},
		{
			name: "test part of group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group2", 1, "cluster3", "cluster4"),
			},
			groupKeys: []GroupKey{
				{GroupName: "group1"},
			},
			expectedExistingClusters:        sets.New[string]("cluster1", "cluster2"),
			expectedExistingBesidesClusters: sets.New[string]("cluster3", "cluster4"),
		},
		{
			name: "test empty group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group2", 1, "cluster3", "cluster4"),
			},
			groupKeys:                       []GroupKey{},
			expectedExistingClusters:        sets.New[string](),
			expectedExistingBesidesClusters: sets.New[string]("cluster1", "cluster2", "cluster3", "cluster4"),
		},
	}

	for _, test := range tests {
		// init fake placement decision getter
		fakeGetter := FakePlacementDecisionGetter{
			FakeDecisions: test.placementDecisions,
		}
		// init tracker
		tracker := NewPlacementDecisionClustersTrackerWithGroups(&test.placement, &fakeGetter, nil)
		err := tracker.Refresh()
		if err != nil {
			t.Errorf("Case: %v, Failed to run Refresh(): %v", test.name, err)
		}

		// Call the Existing method with different groupKeys inputs.
		existingClusters := tracker.ExistingClusterGroups(test.groupKeys...).GetClusters()
		existingBesidesClusters := tracker.ExistingClusterGroupsBesides(test.groupKeys...).GetClusters()

		// Assert the existingClusters
		if !test.expectedExistingClusters.Equal(existingClusters) {
			t.Errorf("Expected: %v, Actual: %v", test.expectedExistingClusters.UnsortedList(), existingClusters.UnsortedList())
		}
		if !test.expectedExistingBesidesClusters.Equal(existingBesidesClusters) {
			t.Errorf("Expected: %v, Actual: %v", test.expectedExistingBesidesClusters.UnsortedList(), existingBesidesClusters.UnsortedList())
		}
	}
}

func TestPlacementDecisionClustersTracker_ExistingClusterGroups(t *testing.T) {
	tests := []struct {
		name                                 string
		placement                            clusterv1beta1.Placement
		placementDecisions                   []*clusterv1beta1.PlacementDecision
		groupKeys                            []GroupKey
		expectedGroupKeys                    []GroupKey
		expectedExistingClusterGroups        map[GroupKey]sets.Set[string]
		expectedBesidesGroupKeys             []GroupKey
		expectedExistingBesidesClusterGroups map[GroupKey]sets.Set[string]
	}{
		{
			name: "test full group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group1", 1, "cluster3", "cluster4"),
				newFakePlacementDecision("placement1", "group2", 2, "cluster5", "cluster6"),
			},
			groupKeys: []GroupKey{
				{GroupName: "group1"},
				{GroupIndex: 2},
				{GroupName: "group3"},
			},
			expectedGroupKeys: []GroupKey{
				{GroupName: "group1", GroupIndex: 0},
				{GroupName: "group1", GroupIndex: 1},
				{GroupName: "group2", GroupIndex: 2},
			},
			expectedExistingClusterGroups: map[GroupKey]sets.Set[string]{
				{GroupName: "group1", GroupIndex: 0}: sets.New[string]("cluster1", "cluster2"),
				{GroupName: "group1", GroupIndex: 1}: sets.New[string]("cluster3", "cluster4"),
				{GroupName: "group2", GroupIndex: 2}: sets.New[string]("cluster5", "cluster6"),
			},
			expectedBesidesGroupKeys:             []GroupKey{},
			expectedExistingBesidesClusterGroups: map[GroupKey]sets.Set[string]{},
		},
		{
			name: "test part of group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group1", 1, "cluster3", "cluster4"),
				newFakePlacementDecision("placement1", "group2", 2, "cluster5", "cluster6"),
			},
			groupKeys: []GroupKey{
				{GroupName: "group1"},
			},
			expectedGroupKeys: []GroupKey{
				{GroupName: "group1", GroupIndex: 0},
				{GroupName: "group1", GroupIndex: 1},
			},
			expectedExistingClusterGroups: map[GroupKey]sets.Set[string]{
				{GroupName: "group1", GroupIndex: 0}: sets.New[string]("cluster1", "cluster2"),
				{GroupName: "group1", GroupIndex: 1}: sets.New[string]("cluster3", "cluster4"),
			},
			expectedBesidesGroupKeys: []GroupKey{
				{GroupName: "group2", GroupIndex: 2},
			},
			expectedExistingBesidesClusterGroups: map[GroupKey]sets.Set[string]{
				{GroupName: "group2", GroupIndex: 2}: sets.New[string]("cluster5", "cluster6"),
			},
		},
		{
			name: "test empty group key",
			placement: clusterv1beta1.Placement{
				ObjectMeta: metav1.ObjectMeta{Name: "placement1", Namespace: "default"},
				Spec:       clusterv1beta1.PlacementSpec{},
			},
			placementDecisions: []*clusterv1beta1.PlacementDecision{
				newFakePlacementDecision("placement1", "group1", 0, "cluster1", "cluster2"),
				newFakePlacementDecision("placement1", "group1", 1, "cluster3", "cluster4"),
				newFakePlacementDecision("placement1", "group2", 2, "cluster5", "cluster6"),
			},
			groupKeys:                     []GroupKey{},
			expectedGroupKeys:             []GroupKey{},
			expectedExistingClusterGroups: map[GroupKey]sets.Set[string]{},
			expectedBesidesGroupKeys: []GroupKey{
				{GroupName: "group1", GroupIndex: 0},
				{GroupName: "group1", GroupIndex: 1},
				{GroupName: "group2", GroupIndex: 2},
			},
			expectedExistingBesidesClusterGroups: map[GroupKey]sets.Set[string]{
				{GroupName: "group1", GroupIndex: 0}: sets.New[string]("cluster1", "cluster2"),
				{GroupName: "group1", GroupIndex: 1}: sets.New[string]("cluster3", "cluster4"),
				{GroupName: "group2", GroupIndex: 2}: sets.New[string]("cluster5", "cluster6"),
			},
		},
	}

	for _, test := range tests {
		// init fake placement decision getter
		fakeGetter := FakePlacementDecisionGetter{
			FakeDecisions: test.placementDecisions,
		}
		// init tracker
		tracker := NewPlacementDecisionClustersTracker(&test.placement, &fakeGetter, nil)
		err := tracker.Refresh()
		if err != nil {
			t.Errorf("Case: %v, Failed to run Refresh(): %v", test.name, err)
		}

		// Call the Existing method with different groupKeys inputs.
		existingClusterGroups := tracker.ExistingClusterGroups(test.groupKeys...)
		existingBesidesClusterGroups := tracker.ExistingClusterGroupsBesides(test.groupKeys...)
		existingGroupKeys := existingClusterGroups.GetOrderedGroupKeys()
		existingBesidesGroupKeys := existingBesidesClusterGroups.GetOrderedGroupKeys()

		// Assert the existingClustersGroups
		if !reflect.DeepEqual(existingGroupKeys, test.expectedGroupKeys) {
			t.Errorf("Expected: %v, Actual: %v", test.expectedGroupKeys, existingGroupKeys)
		}
		for _, gk := range existingGroupKeys {
			if !test.expectedExistingClusterGroups[gk].Equal(existingClusterGroups[gk]) {
				t.Errorf("Expected: %v, Actual: %v", test.expectedExistingClusterGroups[gk], existingClusterGroups[gk])
			}
		}

		if !reflect.DeepEqual(existingBesidesGroupKeys, test.expectedBesidesGroupKeys) {
			t.Errorf("Expected: %v, Actual: %v", test.expectedBesidesGroupKeys, existingBesidesGroupKeys)
		}
		for _, gk := range existingBesidesGroupKeys {
			if !test.expectedExistingBesidesClusterGroups[gk].Equal(existingBesidesClusterGroups[gk]) {
				t.Errorf("Expected: %v, Actual: %v", test.expectedExistingBesidesClusterGroups[gk], existingClusterGroups[gk])
			}
		}
	}
}
