package addon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/protocol/gochan"
	jsonpatch "github.com/evanphx/json-patch/v5"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	addonapiv1alpha1 "open-cluster-management.io/api/addon/v1alpha1"
	addoninformers "open-cluster-management.io/api/client/addon/informers/externalversions"

	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/statushash"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/clients/store"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic"
	"open-cluster-management.io/sdk-go/pkg/cloudevents/generic/options/fake"
)

func TestPatch(t *testing.T) {
	cases := []struct {
		name        string
		clusterName string
		addon       *addonapiv1alpha1.ManagedClusterAddOn
		patch       []byte
	}{
		{
			name:        "patch addon",
			clusterName: "cluster1",
			addon: &addonapiv1alpha1.ManagedClusterAddOn{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "cluster1"},
			},
			patch: func() []byte {
				old := &addonapiv1alpha1.ManagedClusterAddOn{
					ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "cluster1"},
				}
				oldData, err := json.Marshal(old)
				if err != nil {
					t.Error(err)
				}

				new := old.DeepCopy()
				new.Status = addonapiv1alpha1.ManagedClusterAddOnStatus{
					Namespace: "install",
				}

				newData, err := json.Marshal(new)
				if err != nil {
					t.Error(err)
				}

				patchBytes, err := jsonpatch.CreateMergePatch(oldData, newData)
				if err != nil {
					t.Error(err)
				}
				return patchBytes
			}(),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			watcherStore := store.NewAgentInformerWatcherStore[*addonapiv1alpha1.ManagedClusterAddOn]()

			ceClientOpt := fake.NewAgentOptions(gochan.New(), nil, c.clusterName, c.clusterName+"agent")
			ceClient, err := generic.NewCloudEventAgentClient(
				context.Background(),
				ceClientOpt,
				store.NewAgentWatcherStoreLister(watcherStore),
				statushash.StatusHash,
				NewManagedClusterAddOnCodec())
			if err != nil {
				t.Error(err)
			}
			addonClientSet := &AddonClientSetWrapper{&AddonV1Alpha1ClientWrapper{
				NewManagedClusterAddOnClient(ceClient, watcherStore),
			}}

			addonInformerFactory := addoninformers.NewSharedInformerFactory(addonClientSet, time.Minute*10)
			informer := addonInformerFactory.Addon().V1alpha1().ManagedClusterAddOns().Informer()
			store := informer.GetStore()
			if err := store.Add(c.addon); err != nil {
				t.Error(err)
			}
			watcherStore.SetInformer(informer)

			if _, err = addonClientSet.AddonV1alpha1().ManagedClusterAddOns(c.clusterName).Patch(
				context.Background(),
				c.addon.Name,
				types.MergePatchType,
				c.patch,
				metav1.PatchOptions{},
				"status",
			); err != nil {
				t.Error(err)
			}
		})
	}
}
