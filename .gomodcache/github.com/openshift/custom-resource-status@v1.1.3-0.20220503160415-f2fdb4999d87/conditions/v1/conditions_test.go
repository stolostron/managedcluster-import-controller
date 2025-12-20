package v1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSetStatusCondition(t *testing.T) {
	testCases := []struct {
		name               string
		testCondition      Condition
		startConditions    *[]Condition
		expectedConditions *[]Condition
		expectChanged      bool
	}{
		{
			name: "add when empty",
			testCondition: Condition{
				Type:    ConditionAvailable,
				Status:  "True",
				Reason:  "Testing",
				Message: "Basic message",
			},
			startConditions: &[]Condition{},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionAvailable,
					Status:  "True",
					Reason:  "Testing",
					Message: "Basic message",
				},
			},
			expectChanged: true,
		},
		{
			name: "add to conditions",
			testCondition: Condition{
				Type:    ConditionAvailable,
				Status:  "True",
				Reason:  "TestingAvailableTrue",
				Message: "Available condition true",
			},
			startConditions: &[]Condition{
				{
					Type:              ConditionDegraded,
					Status:            "False",
					Reason:            "TestingDegradedFalse",
					Message:           "Degraded condition false",
					LastHeartbeatTime: metav1.NewTime(time.Now()),
				},
			},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionAvailable,
					Status:  "True",
					Reason:  "TestingAvailableTrue",
					Message: "Available condition true",
				},
				{
					Type:    ConditionDegraded,
					Status:  "False",
					Reason:  "TestingDegradedFalse",
					Message: "Degraded condition false",
				},
			},
			expectChanged: true,
		},
		{
			name: "replace condition",
			testCondition: Condition{
				Type:    ConditionDegraded,
				Status:  "True",
				Reason:  "TestingDegradedTrue",
				Message: "Degraded condition true",
			},
			startConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "False",
					Reason:  "TestingDegradedFalse",
					Message: "Degraded condition false",
				},
			},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "True",
					Reason:  "TestingDegradedTrue",
					Message: "Degraded condition true",
				},
			},
			expectChanged: true,
		},
		{
			name: "last heartbeat",
			testCondition: Condition{
				Type:    ConditionDegraded,
				Status:  "True",
				Reason:  "TestingDegradedTrue",
				Message: "Degraded condition true",
			},
			startConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "True",
					Reason:  "TestingDegradedFalse",
					Message: "Degraded condition false",
				},
			},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "True",
					Reason:  "TestingDegradedTrue",
					Message: "Degraded condition true",
				},
			},
			expectChanged: true,
		},
		{
			name: "no change",
			testCondition: Condition{
				Type:    ConditionDegraded,
				Status:  "True",
				Reason:  "TestingDegradedTrue",
				Message: "Degraded condition true",
			},
			startConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "True",
					Reason:  "TestingDegradedTrue",
					Message: "Degraded condition true",
				},
			},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "True",
					Reason:  "TestingDegradedTrue",
					Message: "Degraded condition true",
				},
			},
			expectChanged: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Copy tc.startConditions so it doesn't get updated in place
			startConditions := make([]Condition, len(*tc.startConditions))
			copy(startConditions, *tc.startConditions)
			changed := SetStatusCondition(&startConditions, tc.testCondition)
			if changed != tc.expectChanged {
				t.Errorf("Unexpected return from SetStatusCondition: expected: %t; actual: %t", tc.expectChanged, changed)
			}
			compareConditions(t, &startConditions, tc.expectedConditions)

			// reset
			startConditions = make([]Condition, len(*tc.startConditions))
			copy(startConditions, *tc.startConditions)
			changed = SetStatusConditionNoHeartbeat(&startConditions, tc.testCondition)
			if changed != tc.expectChanged {
				t.Errorf("Unexpected return from SetStatusConditionNoHeartbeat: expected: %t; actual: %t", tc.expectChanged, changed)
			}
			compareConditionsNoHeartbeat(t, &startConditions, tc.expectedConditions)
		})
	}
}

func TestRemoveStatusCondition(t *testing.T) {
	testCases := []struct {
		name               string
		testConditionType  ConditionType
		startConditions    *[]Condition
		expectedConditions *[]Condition
	}{
		{
			name:               "remove when empty",
			testConditionType:  ConditionAvailable,
			startConditions:    &[]Condition{},
			expectedConditions: &[]Condition{},
		},
		{
			name:              "basic remove",
			testConditionType: ConditionAvailable,
			startConditions: &[]Condition{
				{
					Type:              ConditionAvailable,
					Status:            "True",
					Reason:            "TestingAvailableTrue",
					Message:           "Available condition true",
					LastHeartbeatTime: metav1.NewTime(time.Now()),
				},
				{
					Type:              ConditionDegraded,
					Status:            "False",
					Reason:            "TestingDegradedFalse",
					Message:           "Degraded condition false",
					LastHeartbeatTime: metav1.NewTime(time.Now()),
				},
			},
			expectedConditions: &[]Condition{
				{
					Type:    ConditionDegraded,
					Status:  "False",
					Reason:  "TestingDegradedFalse",
					Message: "Degraded condition false",
				},
			},
		},
		{
			name:              "remove last condition",
			testConditionType: ConditionAvailable,
			startConditions: &[]Condition{
				{
					Type:    ConditionAvailable,
					Status:  "True",
					Reason:  "TestingAvailableTrue",
					Message: "Available condition true",
				},
			},
			expectedConditions: &[]Condition{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			RemoveStatusCondition(tc.startConditions, tc.testConditionType)
			compareConditions(t, tc.startConditions, tc.expectedConditions)
		})
	}
}

func compareConditions(t *testing.T, gotConditions *[]Condition, expectedConditions *[]Condition) {
	for _, expectedCondition := range *expectedConditions {
		testCondition := FindStatusCondition(*gotConditions, expectedCondition.Type)
		if testCondition == nil {
			t.Errorf("Condition type '%v' not found in '%v'", expectedCondition.Type, *gotConditions)
		}
		compareCondition(t, testCondition, expectedCondition)
	}
}

func compareCondition(t *testing.T, testCondition *Condition, expectedCondition Condition) {
	compareConditionNoHeartbeat(t, testCondition, expectedCondition)
	// Test for lastHeartbeatTime
	if testCondition.LastHeartbeatTime.IsZero() {
		t.Error("lastHeartbeatTime should never be zero")
	}
	timeNow := metav1.NewTime(time.Now())
	if timeNow.Before(&testCondition.LastHeartbeatTime) {
		t.Errorf("Unexpected lastHeartbeatTime '%v', should be before '%v'", testCondition.LastHeartbeatTime, timeNow)
	}
}

func compareConditionsNoHeartbeat(t *testing.T, gotConditions *[]Condition, expectedConditions *[]Condition) {
	for _, expectedCondition := range *expectedConditions {
		testCondition := FindStatusCondition(*gotConditions, expectedCondition.Type)
		if testCondition == nil {
			t.Errorf("Condition type '%v' not found in '%v'", expectedCondition.Type, *gotConditions)
		}
		compareConditionNoHeartbeat(t, testCondition, expectedCondition)
	}
}

func compareConditionNoHeartbeat(t *testing.T, testCondition *Condition, expectedCondition Condition) {
	if testCondition.Status != expectedCondition.Status {
		t.Errorf("Unexpected status '%v', expected '%v'", testCondition.Status, expectedCondition.Status)
	}
	if testCondition.Message != expectedCondition.Message {
		t.Errorf("Unexpected message '%v', expected '%v'", testCondition.Message, expectedCondition.Message)
	}
	if testCondition.Reason != expectedCondition.Reason {
		t.Errorf("Unexpected reason '%v', expected '%v'", testCondition.Reason, expectedCondition.Reason)
	}

}
