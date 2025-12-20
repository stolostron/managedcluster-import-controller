package errors

import (
	"encoding/json"
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/api/errors"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/common"
)

func TestToStatusError(t *testing.T) {
	cases := []struct {
		name          string
		err           error
		expectErrCode int32
		expectErrMsg  string
	}{
		{
			name:          "grpc error",
			err:           status.Error(codes.Unavailable, "grpc server is unavailable"),
			expectErrCode: 500,
			expectErrMsg:  "Failed to publish resource test: rpc error: code = Unavailable desc = grpc server is unavailable",
		},
		{
			name: "grpc error with a status error",
			err: func() error {
				statusErr := errors.NewNotFound(common.ManagedClusterGR, "test")
				data, err := json.Marshal(statusErr)
				if err != nil {
					t.Fatal(err)
				}
				return status.Error(codes.FailedPrecondition, string(data))
			}(),
			expectErrCode: 404,
			expectErrMsg:  "managedclusters.cluster.open-cluster-management.io \"test\" not found",
		},
		{
			name:          "mqtt error",
			err:           fmt.Errorf("failed to publish event by mqtt"),
			expectErrCode: 500,
			expectErrMsg:  "Failed to publish resource test: failed to publish event by mqtt",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			statusErr := ToStatusError(common.ManagedClusterGR, "test", c.err)
			if statusErr.ErrStatus.Code != c.expectErrCode {
				t.Errorf("expect error code %d, but got %v", c.expectErrCode, statusErr)
			}
			if statusErr.ErrStatus.Message != c.expectErrMsg {
				t.Errorf("expect error msg %s, but got %v", c.expectErrMsg, statusErr)
			}
		})
	}
}

func TestPublishError(t *testing.T) {
	err := NewPublishError(common.ManagedClusterGR, "test", fmt.Errorf("failed to publish resource"))
	if !IsPublishError(err) {
		t.Errorf("expected publish error, but failed")
	}
}
