package authn

import (
	"context"
	"fmt"
	"google.golang.org/grpc/metadata"
	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"testing"
)

func TestTokenAuthenticator(t *testing.T) {
	tests := []struct {
		name     string
		metadata metadata.MD
		token    string
		valid    bool
	}{
		{
			name:     "no authorization field",
			metadata: metadata.MD{},
			valid:    false,
		},
		{
			name: "token is not correct",
			metadata: metadata.MD{
				"Authorization": []string{"Bearer foo"},
			},
			token: "bar",
			valid: false,
		},
		{
			name: "authorization header is set",
			metadata: metadata.MD{
				"Authorization": []string{"Bearer foo"},
			},
			token: "foo",
			valid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := metadata.NewIncomingContext(context.Background(), test.metadata)
			client := fake.NewClientset()
			client.PrependReactor("create", "tokenreviews", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				createAction := action.(clienttesting.CreateAction)
				tr, ok := createAction.GetObject().(*authenticationv1.TokenReview)
				if !ok {
					return false, nil, fmt.Errorf("not a TokenReview")
				}
				if tr.Spec.Token != test.token {
					return false, nil, fmt.Errorf("invalid token")
				}
				tr.Status = authenticationv1.TokenReviewStatus{Authenticated: true}
				return true, tr, nil
			})
			authenticator := NewTokenAuthenticator(client)
			_, err := authenticator.Authenticate(ctx)
			if test.valid {
				if err != nil {
					t.Errorf("authenticator.Authenticate() = %v", err)
				}

			}
			if !test.valid && err == nil {
				t.Errorf("authenticator.Authenticate() = %v, wanted error", err)
			}
		})
	}
}
