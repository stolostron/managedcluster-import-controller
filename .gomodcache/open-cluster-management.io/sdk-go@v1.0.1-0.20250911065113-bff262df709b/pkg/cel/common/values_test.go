package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type testObject struct {
	Field1 string `json:"field1"`
	Field2 int    `json:"field2"`
}

func TestConvertObjectToUnstructured(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    *unstructured.Unstructured
		wantErr bool
	}{
		{
			name:  "nil input",
			input: nil,
			want:  &unstructured.Unstructured{Object: nil},
		},
		{
			name:  "nil pointer",
			input: (*testObject)(nil),
			want:  &unstructured.Unstructured{Object: nil},
		},
		{
			name: "valid object",
			input: &testObject{
				Field1: "test",
				Field2: 42,
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"field1": "test",
					"field2": int64(42),
				},
			},
		},
		{
			name: "already unstructured",
			input: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"field1": "test",
					"field2": int64(42),
				},
			},
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"field1": "test",
					"field2": int64(42),
				},
			},
		},
		{
			name:    "invalid object",
			input:   func() {},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ConvertObjectToUnstructured(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
