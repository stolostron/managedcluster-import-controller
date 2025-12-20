package library

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/sets"
	clusterlisterv1alpha1 "open-cluster-management.io/api/client/cluster/listers/cluster/v1alpha1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	clusterapiv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	"open-cluster-management.io/sdk-go/pkg/cel/common"
	testinghelpers "open-cluster-management.io/sdk-go/pkg/testing"
)

func TestManagedCluster(t *testing.T) {
	trueVal := types.Bool(true)
	falseVal := types.Bool(false)

	cases := []struct {
		name                string
		expr                string
		cluster             *clusterapiv1.ManagedCluster
		expectValue         ref.Val
		expectedCompileErrs []string
		expectedRuntimeErr  string
	}{
		{
			name:        "match with label",
			expr:        `managedCluster.metadata.labels["version"].matches('^1\\.(14|15)\\.\\d+$')`,
			cluster:     testinghelpers.NewManagedCluster("test").WithLabel("version", "1.14.3").Build(),
			expectValue: trueVal,
		},
		{
			name:        "not match with label",
			expr:        `managedCluster.metadata.labels["version"].matches('^1\\.(14|15)\\.\\d+$')`,
			cluster:     testinghelpers.NewManagedCluster("test").WithLabel("version", "1.16.3").Build(),
			expectValue: falseVal,
		},
		{
			name:                "invalid labels expression",
			expr:                `managedCluster.metadata.labels["version"].matchessssss('^1\\.(14|15)\\.\\d+$')`,
			cluster:             testinghelpers.NewManagedCluster("test").WithLabel("version", "1.14.3").Build(),
			expectValue:         falseVal,
			expectedCompileErrs: []string{"undeclared reference to 'matchessssss'"},
		},
		{
			name:        "match with claim",
			expr:        `managedCluster.status.clusterClaims.exists(c, c.name == "version" && c.value.matches('^1\\.(14|15)\\.\\d+$'))`,
			cluster:     testinghelpers.NewManagedCluster("test").WithClaim("version", "1.14.3").Build(),
			expectValue: trueVal,
		},
		{
			name:        "not match with claim",
			expr:        `managedCluster.status.clusterClaims.exists(c, c.name == "version" && c.value.matches('^1\\.(14|15)\\.\\d+$'))`,
			cluster:     testinghelpers.NewManagedCluster("test").WithClaim("version", "1.16.3").Build(),
			expectValue: falseVal,
		},
		{
			name:                "invalid claims expression",
			expr:                `managedCluster.status.clusterClaims.exists(c, c.name == "version" && c.value.matchessssss('^1\\.(14|15)\\.\\d+$'))`,
			cluster:             testinghelpers.NewManagedCluster("test").WithClaim("version", "1.14.3").Build(),
			expectValue:         falseVal,
			expectedCompileErrs: []string{"undeclared reference to 'matchessssss'"},
		},
		{
			name:        "match with score value",
			expr:        `managedCluster.scores("test-score").filter(s, s.name == 'cpu').all(e, e.value == 3)`,
			cluster:     testinghelpers.NewManagedCluster("test").Build(),
			expectValue: trueVal,
		},
		{
			name:        "not match with score value",
			expr:        `managedCluster.scores("test-score").filter(s, s.name == 'cpu').all(e, e.value > 3)`,
			cluster:     testinghelpers.NewManagedCluster("test").Build(),
			expectValue: falseVal,
		},
		{
			name:               "invalid score name",
			expr:               `managedCluster.scores("invalid-score").filter(s, s.name == 'cpu')`,
			cluster:            testinghelpers.NewManagedCluster("test").Build(),
			expectValue:        falseVal,
			expectedRuntimeErr: "failed to get score: invaild score name invalid-score",
		},
		{
			name:        "match with json",
			expr:        `managedCluster.status.clusterClaims.exists(c, c.name == "sku.gpu.kubernetes-fleet.io" && c.value.parseJSON().H100.exists(e, e.Standard_NC96ads_H100_v4 == 2))`,
			cluster:     testinghelpers.NewManagedCluster("test").WithClaim("sku.gpu.kubernetes-fleet.io", "{\"H100\":[{\"Standard_NC48ads_H100_v4\":10},{\"Standard_NC96ads_H100_v4\":2}],\"A100\":[{\"Standard_NC48ads_A100_v4\":50},{\"Standard_NC96ads_A100_v4\":20}]}").Build(),
			expectValue: trueVal,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testManagedCluster(t, c.expr, c.cluster, c.expectValue, c.expectedRuntimeErr, c.expectedCompileErrs)
		})
	}
}

func testManagedCluster(t *testing.T, expr string, cluster *clusterapiv1.ManagedCluster, expectValue ref.Val, expectRuntimeErrPattern string, expectCompileErrs []string) {
	envOpts := append([]cel.EnvOption{ManagedClusterLib(&fakeScoreLister{}), JsonLib()}, common.BaseEnvOpts...)
	env, err := cel.NewEnv(envOpts...)
	if err != nil {
		t.Fatalf("%v", err)
	}

	compiled, issues := env.Compile(expr)

	if len(expectCompileErrs) > 0 {
		missingCompileErrs := []string{}
		matchedCompileErrs := sets.New[int]()
		for _, expectedCompileErr := range expectCompileErrs {
			compiledPattern, err := regexp.Compile(expectedCompileErr)
			if err != nil {
				t.Fatalf("failed to compile expected err regex: %v", err)
			}

			didMatch := false

			for i, compileError := range issues.Errors() {
				if compiledPattern.Match([]byte(compileError.Message)) {
					didMatch = true
					matchedCompileErrs.Insert(i)
				}
			}

			if !didMatch {
				missingCompileErrs = append(missingCompileErrs, expectedCompileErr)
			} else if len(matchedCompileErrs) != len(issues.Errors()) {
				unmatchedErrs := []cel.Error{}
				for i, issue := range issues.Errors() {
					if !matchedCompileErrs.Has(i) {
						unmatchedErrs = append(unmatchedErrs, *issue)
					}
				}
				require.Empty(t, unmatchedErrs, "unexpected compilation errors")
			}
		}

		require.Empty(t, missingCompileErrs, "expected compilation errors")
		return
	} else if len(issues.Errors()) > 0 {
		t.Fatalf("%v", issues.Errors())
	}

	convertedCluster, err := common.ConvertObjectToUnstructured(cluster)
	if err != nil {
		t.Fatalf("%v", err)
	}

	prog, err := env.Program(compiled)
	if err != nil {
		t.Fatalf("%v", err)
	}
	res, _, err := prog.Eval(map[string]interface{}{
		"managedCluster": convertedCluster.Object,
	})
	if len(expectRuntimeErrPattern) > 0 {
		if err == nil {
			t.Fatalf("no runtime error thrown. Expected: %v", expectRuntimeErrPattern)
		} else if expectRuntimeErrPattern != err.Error() {
			t.Fatalf("unexpected err: %v", err)
		}
	} else if err != nil {
		t.Fatalf("%v", err)
	} else if expectValue != nil {
		converted := res.Equal(expectValue).Value().(bool)
		require.True(t, converted, "expectation not equal to output")
	} else {
		t.Fatal("expected result must not be nil")
	}
}

type fakeScoreLister struct {
	namespace string
}

func (f *fakeScoreLister) AddOnPlacementScores(namespace string) clusterlisterv1alpha1.AddOnPlacementScoreNamespaceLister {
	f.namespace = namespace
	return f
}

// Get returns a fake score with predefined values
func (f *fakeScoreLister) Get(name string) (*clusterapiv1alpha1.AddOnPlacementScore, error) {
	if name == "test-score" {
		return &clusterapiv1alpha1.AddOnPlacementScore{
			Status: clusterapiv1alpha1.AddOnPlacementScoreStatus{
				Scores: []clusterapiv1alpha1.AddOnPlacementScoreItem{
					{Name: "cpu", Value: 3},
					{Name: "memory", Value: 4},
				},
			},
		}, nil
	} else {
		return nil, fmt.Errorf("invaild score name %s", name)
	}
}

// List returns empty list since it's not used in tests
func (f *fakeScoreLister) List(selector labels.Selector) ([]*clusterapiv1alpha1.AddOnPlacementScore, error) {
	return []*clusterapiv1alpha1.AddOnPlacementScore{}, nil
}
