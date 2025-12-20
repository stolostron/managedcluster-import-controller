package v1alpha1

import "testing"

// Test cases for ScoreNormalizer.
func TestScoreNormalizer(t *testing.T) {
	normalizer := NewScoreNormalizer(0, 50)

	tests := []struct {
		value    float64
		expected int32
	}{
		{value: 0, expected: -100},
		{value: 25, expected: 0},
		{value: 50, expected: 100},
		{value: -10, expected: -100},
		{value: 60, expected: 100},
	}

	for _, test := range tests {
		score, err := normalizer.Normalize(test.value)
		if err != nil {
			t.Errorf("Unexpected error for value %v: %v", test.value, err)
		}
		if score != test.expected {
			t.Errorf("For value %v, expected score %v, but got %v", test.value, test.expected, score)
		}
	}
}
