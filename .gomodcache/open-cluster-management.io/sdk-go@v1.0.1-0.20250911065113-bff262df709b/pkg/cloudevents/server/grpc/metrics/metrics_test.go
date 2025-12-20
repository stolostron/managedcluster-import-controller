package metrics

import (
	"testing"
)

func TestSplitMethod(t *testing.T) {
	tests := []struct {
		name            string
		fullMethod      string
		expectedService string
		expectedMethod  string
	}{
		{"empty full method", "", "unknown", "unknown"},
		{"no leading slash", "io.cloudevents.v1.CloudEventService/Subscribe", "io.cloudevents.v1.CloudEventService", "Subscribe"},
		{"leading slash", "/io.cloudevents.v1.CloudEventService/Subscribe", "io.cloudevents.v1.CloudEventService", "Subscribe"},
		{"no slash", "io.cloudevents.v1.CloudEventService", "io.cloudevents.v1.CloudEventService", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotService, gotMethod := SplitMethod(tt.fullMethod)
			if gotService != tt.expectedService {
				t.Errorf("splitMethod(%s) gotService = %s, want %s", tt.fullMethod, gotService, tt.expectedService)
			}
			if gotMethod != tt.expectedMethod {
				t.Errorf("splitMethod(%s) gotMethod = %s, want %s", tt.fullMethod, gotMethod, tt.expectedMethod)
			}
		})
	}
}
