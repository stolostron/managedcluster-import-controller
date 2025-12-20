package csr

import (
	"context"
	"testing"
	"time"

	v1 "open-cluster-management.io/api/cluster/v1"

	"github.com/cloudevents/sdk-go/v2/protocol/gochan"

	certificatev1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/statushash"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
)

func TestCreate(t *testing.T) {
	cases := []struct {
		name        string
		csrs        []runtime.Object
		csr         *certificatev1.CertificateSigningRequest
		expectedErr string
	}{
		{
			name: "new cluster",
			csrs: []runtime.Object{},
			csr: &certificatev1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						v1.ClusterNameLabelKey: "cluster1",
					},
				},
			},
			expectedErr: "",
		},
		{
			name: "existing cluster",
			csrs: []runtime.Object{&certificatev1.CertificateSigningRequest{ObjectMeta: metav1.ObjectMeta{Name: "cluster1"}}},
			csr: &certificatev1.CertificateSigningRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster1",
					Labels: map[string]string{
						v1.ClusterNameLabelKey: "cluster1",
					},
				},
			},
			expectedErr: "certificatesigningrequests.certificates.k8s.io \"cluster1\" already exists",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			watcherStore := store.NewAgentInformerWatcherStore[*certificatev1.CertificateSigningRequest]()
			ceClientOpt := fake.NewAgentOptions(gochan.New(), nil, "cluster1", "cluster1-agent")
			ceClient, err := generic.NewCloudEventAgentClient(
				ctx,
				ceClientOpt,
				store.NewAgentWatcherStoreLister(watcherStore),
				statushash.StatusHash,
				NewCSRCodec())
			if err != nil {
				t.Error(err)
			}

			csrClient := NewCSRClient(ceClient, watcherStore, "cluster1")
			csrInformer := cache.NewSharedIndexInformer(
				csrClient, &certificatev1.CertificateSigningRequest{}, 30*time.Second,
				cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc})
			csrInformerStore := csrInformer.GetStore()
			for _, csr := range c.csrs {
				if err := csrInformerStore.Add(csr); err != nil {
					t.Error(err)
				}
			}
			watcherStore.SetInformer(csrInformer)
			go csrInformer.Run(ctx.Done())

			if _, err := csrClient.Create(ctx, c.csr, metav1.CreateOptions{}); err != nil {
				if c.expectedErr == err.Error() {
					return
				}

				t.Error(err)
			}
		})
	}
}
