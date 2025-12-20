package library

import (
	"math"
	"testing"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
	ocmcelcommon "open-cluster-management.io/sdk-go/pkg/cel/common"
	testinghelpers "open-cluster-management.io/sdk-go/pkg/testing"
)

func TestLibraryCost(t *testing.T) {
	cases := []struct {
		name              string
		expr              string
		cluster           *clusterapiv1.ManagedCluster
		expectRuntimeCost uint64
	}{
		// Test cases for scores function
		{
			name:              "scores_empty",
			expr:              `managedCluster.scores("fake-score")`,
			cluster:           testinghelpers.NewManagedCluster("test").Build(),
			expectRuntimeCost: common.ListCreateBaseCost + common.SelectAndIdentCost, // 11
		},
		{
			name:              "scores_with_items",
			expr:              `managedCluster.scores("test-score")`,
			cluster:           testinghelpers.NewManagedCluster("test").Build(),
			expectRuntimeCost: common.ListCreateBaseCost + 2*(common.SelectAndIdentCost+common.MapCreateBaseCost) + common.SelectAndIdentCost, // 73
		},
		// Test cases for parseJSON function
		{
			name:              "parse empty string",
			expr:              `''.parseJSON()`,
			expectRuntimeCost: common.ConstCost, // 0
		},
		{
			name:              "parse simple string",
			expr:              `'hello'.parseJSON()`,
			expectRuntimeCost: uint64(math.Ceil(5 * common.StringTraversalCostFactor)), // 1
		},
		{
			name:              "parse simple object",
			expr:              `'{"key":"value"}'.parseJSON()`,
			expectRuntimeCost: uint64(math.Ceil(15*common.StringTraversalCostFactor)) + common.MapCreateBaseCost, // 32
		},
		{
			name:              "parse nested object",
			expr:              `'{"nested": {"key": "value"}}'.parseJSON()`,
			expectRuntimeCost: uint64(math.Ceil(28*common.StringTraversalCostFactor)) + 2*common.MapCreateBaseCost, // 63
		},
		{
			name:              "parse array",
			expr:              `'[1,2,3]'.parseJSON()`,
			expectRuntimeCost: uint64(math.Ceil(7*common.StringTraversalCostFactor)) + common.ListCreateBaseCost, // 11
		},
		{
			name:              "parse array of objects",
			expr:              `'[{"id": 1}, {"id": 2}]'.parseJSON()`,
			expectRuntimeCost: uint64(math.Ceil(22*common.StringTraversalCostFactor)) + common.ListCreateBaseCost + 2*common.MapCreateBaseCost, // 73
		},
	}

	estimator := &CostEstimator{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			envOpts := append([]cel.EnvOption{ManagedClusterLib(&fakeScoreLister{}), JsonLib()}, ocmcelcommon.BaseEnvOpts...)
			env, err := cel.NewEnv(envOpts...)
			if err != nil {
				t.Fatalf("%v", err)
			}

			compiled, issues := env.Compile(tc.expr)
			if issues.Err() != nil {
				t.Fatalf("%v", issues.Err())
			}

			convertedCluster, err := ocmcelcommon.ConvertObjectToUnstructured(tc.cluster)
			if err != nil {
				t.Fatalf("%v", err)
			}

			// Test runtime cost
			prog, err := env.Program(compiled,
				cel.CostTracking(estimator),
			)
			if err != nil {
				t.Fatalf("%v", err)
			}

			_, evalDetails, _ := prog.Eval(map[string]interface{}{
				"managedCluster": convertedCluster.Object,
			})

			if evalDetails.ActualCost() == nil {
				t.Fatal("Expected non-nil ActualCost")
			}

			if *evalDetails.ActualCost() != tc.expectRuntimeCost {
				t.Errorf("Runtime cost = %v, want %v", *evalDetails.ActualCost(), tc.expectRuntimeCost)
			}
		})
	}
}

// Helper test for actualSize function
func TestActualSizeCalculation(t *testing.T) {
	cases := []struct {
		name     string
		input    ref.Val
		wantSize uint64
	}{
		{
			name:     "string_size",
			input:    types.String("test"),
			wantSize: 4,
		},
		{
			name:     "empty_string",
			input:    types.String(""),
			wantSize: 0,
		},
		{
			name:     "list_size",
			input:    types.NewStringList(types.DefaultTypeAdapter, []string{"a", "b", "c"}),
			wantSize: 3,
		},
		{
			name:     "empty_list",
			input:    types.NewStringList(types.DefaultTypeAdapter, []string{}),
			wantSize: 0,
		},
		{
			name:     "non_sizer_type",
			input:    types.Bool(true),
			wantSize: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := actualSize(tc.input)
			if got != tc.wantSize {
				t.Errorf("actualSize(%v) = %v, want %v", tc.input, got, tc.wantSize)
			}
		})
	}
}
