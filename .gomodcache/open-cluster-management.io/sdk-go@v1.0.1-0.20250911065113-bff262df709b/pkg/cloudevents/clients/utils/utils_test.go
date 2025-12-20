package utils

import (
	"encoding/json"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/tools/cache"

	workv1 "open-cluster-management.io/api/work/v1"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/common"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
)

func TestPatch(t *testing.T) {
	cases := []struct {
		name      string
		patchType types.PatchType
		work      *workv1.ManifestWork
		patch     []byte
		validate  func(t *testing.T, work *workv1.ManifestWork)
	}{
		{
			name:      "json patch",
			patchType: types.JSONPatchType,
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			patch: []byte("[{\"op\":\"replace\",\"path\":\"/metadata/name\",\"value\":\"test1\"}]"),
			validate: func(t *testing.T, work *workv1.ManifestWork) {
				if work.Name != "test1" {
					t.Errorf("unexpected work %v", work)
				}
			},
		},
		{
			name:      "merge patch",
			patchType: types.MergePatchType,
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			patch: func() []byte {
				newWork := &workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test2",
						Namespace: "test2",
					},
				}
				data, err := json.Marshal(newWork)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}(),
			validate: func(t *testing.T, work *workv1.ManifestWork) {
				if work.Name != "test2" {
					t.Errorf("unexpected work %v", work)
				}
				if work.Namespace != "test2" {
					t.Errorf("unexpected work %v", work)
				}
			},
		},
		{
			name:      "strategic merge patch",
			patchType: types.StrategicMergePatchType,
			work: &workv1.ManifestWork{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test",
				},
			},
			patch: func() []byte {
				oldData, err := json.Marshal(&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test",
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				newData, err := json.Marshal(&workv1.ManifestWork{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test",
						Namespace: "test",
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				data, err := strategicpatch.CreateTwoWayMergePatch(oldData, newData, workv1.ManifestWork{})
				if err != nil {
					t.Fatal(err)
				}
				return data
			}(),
			validate: func(t *testing.T, work *workv1.ManifestWork) {
				if work.Name != "test" {
					t.Errorf("unexpected work %v", work)
				}
				if work.Namespace != "test" {
					t.Errorf("unexpected work %v", work)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			work, err := Patch(c.patchType, c.work, c.patch)
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}

			c.validate(t, work)
		})
	}
}

func TestListWithOptions(t *testing.T) {
	cases := []struct {
		name          string
		works         []*workv1.ManifestWork
		workNamespace string
		opts          metav1.ListOptions
		expectedWorks int
	}{
		{
			name: "list all works",
			works: []*workv1.ManifestWork{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster2",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
			},
			workNamespace: metav1.NamespaceAll,
			opts:          metav1.ListOptions{},
			expectedWorks: 3,
		},
		{
			name: "list works from a given namespace",
			works: []*workv1.ManifestWork{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster2",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
			},
			workNamespace: "cluster1",
			opts:          metav1.ListOptions{},
			expectedWorks: 2,
		},
		{
			name: "list with fields",
			works: []*workv1.ManifestWork{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "false",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster2",
						Labels: map[string]string{
							"test": "false",
						},
					},
				},
			},
			opts: metav1.ListOptions{
				FieldSelector: "metadata.name=t1",
			},
			workNamespace: "cluster1",
			expectedWorks: 1,
		},
		{
			name: "list with labels",
			works: []*workv1.ManifestWork{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "false",
						},
					},
				},
			},
			opts: metav1.ListOptions{
				LabelSelector: "test=true",
			},
			workNamespace: "cluster1",
			expectedWorks: 1,
		},
		{
			name: "list with labels and fields",
			works: []*workv1.ManifestWork{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t1",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "true",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "t2",
						Namespace: "cluster1",
						Labels: map[string]string{
							"test": "false",
						},
					},
				},
			},
			opts: metav1.ListOptions{
				LabelSelector: "test=true",
				FieldSelector: "metadata.name=t1",
			},
			workNamespace: "cluster1",
			expectedWorks: 1,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			store := cache.NewStore(cache.MetaNamespaceKeyFunc)
			for _, work := range c.works {
				if err := store.Add(work); err != nil {
					t.Fatal(err)
				}
			}
			works, err := ListResourcesWithOptions[*workv1.ManifestWork](store, c.workNamespace, c.opts)
			if err != nil {
				t.Errorf("unexpected error %v", err)
			}

			if len(works) != c.expectedWorks {
				t.Errorf("expected %d, but %v", c.expectedWorks, works)
			}
		})
	}
}

func TestValidateResourceMetadata(t *testing.T) {
	cases := []struct {
		name             string
		resource         generic.ResourceObject
		expectedErrorMsg string
	}{
		{
			name:             "no metadata",
			resource:         nil,
			expectedErrorMsg: "metadata: Invalid value: \"null\": object does not implement the Object interfaces",
		},
		{
			name:             "no uid",
			resource:         &metav1.ObjectMeta{},
			expectedErrorMsg: "metadata.uid: Required value: field not set",
		},
		{
			name:             "no ResourceVersion",
			resource:         &metav1.ObjectMeta{UID: types.UID("1")},
			expectedErrorMsg: "metadata.resourceVersion: Required value: field not set",
		},
		{
			name: "no name",
			resource: &metav1.ObjectMeta{
				UID:             types.UID("1"),
				ResourceVersion: "0",
			},
			expectedErrorMsg: "metadata.name: Required value: field not set",
		},
		{
			name: "bad name",
			resource: &metav1.ObjectMeta{
				UID:             types.UID("1"),
				ResourceVersion: "0",
				Name:            "@",
			},
			expectedErrorMsg: "metadata.name: Invalid value: \"@\": a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')",
		},
		{
			name: "bad namespace",
			resource: &metav1.ObjectMeta{
				UID:             types.UID("1"),
				ResourceVersion: "0",
				Name:            "test",
				Namespace:       "@",
			},
			expectedErrorMsg: "metadata.namespace: Invalid value: \"@\": a lowercase RFC 1123 label must consist of lower case alphanumeric characters or '-', and must start and end with an alphanumeric character (e.g. 'my-name',  or '123-abc', regex used for validation is '[a-z0-9]([-a-z0-9]*[a-z0-9])?')",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			errs := ValidateResourceMetadata(c.resource)
			if len(errs) != 0 && errs.ToAggregate().Error() != c.expectedErrorMsg {
				t.Errorf("expected %#v but got: %#v", c.expectedErrorMsg, errs.ToAggregate().Error())
			}
		})
	}
}

func TestUID(t *testing.T) {
	first := UID("source1", common.ManifestWorkGR.String(), "ns", "name")
	second := UID("source1", common.ManifestWorkGR.String(), "ns", "name")
	if first != second {
		t.Errorf("expected two uid equal, but %v, %v", first, second)
	}
}

func TestToRuntimeObject(t *testing.T) {
	cases := []struct {
		name             string
		resource         generic.ResourceObject
		expectedErrorMsg string
	}{
		{
			name:             "not a runtime object",
			resource:         nil,
			expectedErrorMsg: "object <nil> does not implement the runtime Object interfaces",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ToRuntimeObject(c.resource)
			if err != nil && err.Error() != c.expectedErrorMsg {
				t.Errorf("expected %#v but got: %#v", c.expectedErrorMsg, err.Error())
			}
		})
	}
}

func TestCompareSnowflakeSequenceIDs(t *testing.T) {
	cases := []struct {
		name       string
		lastSID    string
		currentSID string
		expected   bool
	}{
		{
			name:       "last sid is empty",
			lastSID:    "",
			currentSID: "1834773391719010304",
			expected:   true,
		},
		{
			name:       "compare two sids",
			lastSID:    "1834773391719010304",
			currentSID: "1834773613329256448",
			expected:   true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			actual, err := CompareSnowflakeSequenceIDs(c.lastSID, c.currentSID)
			if err != nil {
				t.Fatal(err)
			}

			if actual != c.expected {
				t.Errorf("expected %v, but %v", c.expected, actual)
			}

		})
	}
}

func TestEnsureResourceFinalizer(t *testing.T) {
	tests := []struct {
		name       string
		input      []string
		wantOutput []string
	}{
		{
			name:       "empty finalizers",
			input:      []string{},
			wantOutput: []string{common.ResourceFinalizer},
		},
		{
			name:       "finalizer already exists",
			input:      []string{"other-finalizer", common.ResourceFinalizer},
			wantOutput: []string{"other-finalizer", common.ResourceFinalizer},
		},
		{
			name:       "finalizer not present",
			input:      []string{"finalizer1", "finalizer2"},
			wantOutput: []string{"finalizer1", "finalizer2", common.ResourceFinalizer},
		},
		{
			name:       "nil input",
			input:      nil,
			wantOutput: []string{common.ResourceFinalizer},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EnsureResourceFinalizer(tt.input)
			if !reflect.DeepEqual(got, tt.wantOutput) {
				t.Errorf("EnsureFinalizers() = %v, want %v", got, tt.wantOutput)
			}
		})
	}
}
