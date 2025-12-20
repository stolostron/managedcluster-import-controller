package sar

import (
	"context"
	"testing"

	authv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/cluster"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/lease"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/work/payload"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/types"
	"open-cluster-management.io/sdk-go/pkg/server/grpc/authn"
	"open-cluster-management.io/sdk-go/pkg/server/grpc/authz"
)

func TestSARAuthorize(t *testing.T) {
	type testCase struct {
		name       string
		cluster    string
		eventsType types.CloudEventsType
		userCtx    func() context.Context
		allow      func(sar *authv1.SubjectAccessReview) bool
		expectErr  bool
	}

	testCases := []testCase{
		{
			name:    "allowed for cluster creation",
			cluster: "cluster1",
			eventsType: types.CloudEventsType{
				CloudEventsDataType: cluster.ManagedClusterEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              types.CreateRequestAction,
			},
			userCtx: func() context.Context {
				return context.WithValue(context.Background(), authn.ContextUserKey, "test")
			},
			allow: func(sar *authv1.SubjectAccessReview) bool {
				if sar.Spec.User != "test" {
					return false
				}

				if sar.Spec.ResourceAttributes.Group != clusterv1.SchemeGroupVersion.Group ||
					sar.Spec.ResourceAttributes.Resource != "managedclusters" ||
					sar.Spec.ResourceAttributes.Name != "cluster1" ||
					sar.Spec.ResourceAttributes.Namespace != "cluster1" {
					return false
				}

				if sar.Spec.ResourceAttributes.Verb != "create" {
					return false
				}

				return true
			},
			expectErr: false,
		},
		{
			name:    "allowed for manifest status update",
			cluster: "cluster1",
			eventsType: types.CloudEventsType{
				CloudEventsDataType: payload.ManifestBundleEventDataType,
				SubResource:         types.SubResourceStatus,
				Action:              types.UpdateRequestAction,
			},
			userCtx: func() context.Context {
				return context.WithValue(context.Background(), authn.ContextGroupsKey, []string{"group1", "group2"})
			},
			allow: func(sar *authv1.SubjectAccessReview) bool {
				groups := sets.New(sar.Spec.Groups...)
				if !groups.Has("group2") {
					return false
				}

				if sar.Spec.ResourceAttributes.Group != workv1.SchemeGroupVersion.Group ||
					sar.Spec.ResourceAttributes.Resource != "manifestworks" ||
					sar.Spec.ResourceAttributes.Subresource != "status" ||
					sar.Spec.ResourceAttributes.Namespace != "cluster1" {
					return false
				}

				if sar.Spec.ResourceAttributes.Verb != "update" {
					return false
				}

				return true
			},
			expectErr: false,
		},
		{
			name:    "allowed for lease subscription",
			cluster: "cluster1",
			eventsType: types.CloudEventsType{
				CloudEventsDataType: lease.LeaseEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              types.WatchRequestAction,
			},
			userCtx: func() context.Context {
				return context.WithValue(context.Background(), authn.ContextUserKey, "test")
			},
			allow: func(sar *authv1.SubjectAccessReview) bool {
				if sar.Spec.User != "test" {
					return false
				}

				if sar.Spec.ResourceAttributes.Group != coordinationv1.SchemeGroupVersion.Group ||
					sar.Spec.ResourceAttributes.Resource != "leases" ||
					sar.Spec.ResourceAttributes.Namespace != "cluster1" {
					return false
				}

				if sar.Spec.ResourceAttributes.Verb != "watch" {
					return false
				}

				return true
			},
			expectErr: false,
		},
		{
			name:    "allowed for cluster resync",
			cluster: "cluster1",
			eventsType: types.CloudEventsType{
				CloudEventsDataType: cluster.ManagedClusterEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              types.ResyncRequestAction,
			},
			userCtx: func() context.Context {
				return context.WithValue(context.Background(), authn.ContextUserKey, "test")
			},
			allow: func(sar *authv1.SubjectAccessReview) bool {
				if sar.Spec.User != "test" {
					return false
				}

				if sar.Spec.ResourceAttributes.Group != clusterv1.SchemeGroupVersion.Group ||
					sar.Spec.ResourceAttributes.Resource != "managedclusters" ||
					sar.Spec.ResourceAttributes.Name != "cluster1" ||
					sar.Spec.ResourceAttributes.Namespace != "cluster1" {
					return false
				}

				if sar.Spec.ResourceAttributes.Verb != "list" {
					return false
				}

				return true
			},
			expectErr: false,
		},
		{
			name: "denied for cluster deletion",
			eventsType: types.CloudEventsType{
				CloudEventsDataType: cluster.ManagedClusterEventDataType,
				SubResource:         types.SubResourceSpec,
				Action:              types.DeleteRequestAction,
			},
			userCtx: func() context.Context {
				return context.WithValue(context.Background(), authn.ContextUserKey, "test")
			},
			allow: func(sar *authv1.SubjectAccessReview) bool {
				return false
			},
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset()

			client.Fake.PrependReactor(
				"create",
				"subjectaccessreviews",
				func(action clienttesting.Action) (bool, runtime.Object, error) {
					createAction, ok := action.(clienttesting.CreateAction)
					if !ok {
						t.Fatalf("unexpected action %T", action)
					}

					sarObj := createAction.GetObject()
					sar, ok := sarObj.(*authv1.SubjectAccessReview)
					if !ok {
						t.Fatalf("unexpected object %T", sarObj)
					}

					return true, &authv1.SubjectAccessReview{Status: authv1.SubjectAccessReviewStatus{Allowed: tc.allow(sar)}}, nil
				},
			)

			auth := NewSARAuthorizer(client)

			decision, err := auth.authorize(tc.userCtx(), tc.cluster, tc.eventsType)
			if tc.expectErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tc.expectErr && decision != authz.DecisionAllow {
				t.Errorf("expected DecisionAllow, got %v", decision)
			}
			if tc.expectErr && decision != authz.DecisionDeny {
				t.Errorf("expected DecisionDeny, got %v", decision)
			}
		})
	}
}
