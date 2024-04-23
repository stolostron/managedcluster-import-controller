package helpers

import (
	"testing"

	klusterletconfigv1alpha1 "github.com/stolostron/cluster-lifecycle-api/klusterletconfig/v1alpha1"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
)

func TestGetMergedKlusterletConfigWithGlobal(t *testing.T) {
	tests := []struct {
		name                 string
		klusterletconfigName string
		getFunc              func(string) (*klusterletconfigv1alpha1.KlusterletConfig, error)
		wantErr              bool
	}{
		{
			name:                 "return error when klusterletconfigLister.Get returns an error",
			klusterletconfigName: "test",
			getFunc: func(string) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
				return nil, errors.NewServiceUnavailable("unavailable")
			},
			wantErr: true,
		},
		{
			name:                 "return error when klusterletconfigLister.Get for global klusterletconfig returns an error",
			klusterletconfigName: "",
			getFunc: func(name string) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
				if name == constants.GlobalKlusterletConfigName {
					return nil, errors.NewServiceUnavailable("unavailable")
				}
				return &klusterletconfigv1alpha1.KlusterletConfig{}, nil
			},
			wantErr: true,
		},
		{
			name:                 "return merged klusterletconfig when klusterletconfigLister.Get functions succeed",
			klusterletconfigName: "test",
			getFunc: func(name string) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
				return &klusterletconfigv1alpha1.KlusterletConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name: name,
					},
				}, nil
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &mockKlusterletConfigLister{
				GetFunc: tt.getFunc,
			}

			_, err := GetMergedKlusterletConfigWithGlobal(tt.klusterletconfigName, lister)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("expected nil error, got %v", err)
				}
			}
		})
	}
}

// mockKlusterletConfigLister is a mock implementation of KlusterletConfigLister interface.
type mockKlusterletConfigLister struct {
	ListFunc func(selector labels.Selector) ([]*klusterletconfigv1alpha1.KlusterletConfig, error)
	GetFunc  func(name string) (*klusterletconfigv1alpha1.KlusterletConfig, error)
}

// List lists all KlusterletConfigs in the indexer.
func (m *mockKlusterletConfigLister) List(selector labels.Selector) ([]*klusterletconfigv1alpha1.KlusterletConfig, error) {
	if m.ListFunc != nil {
		return m.ListFunc(selector)
	}
	return nil, nil
}

// Get retrieves the KlusterletConfig from the index for a given name.
func (m *mockKlusterletConfigLister) Get(name string) (*klusterletconfigv1alpha1.KlusterletConfig, error) {
	if m.GetFunc != nil {
		return m.GetFunc(name)
	}
	return nil, nil
}
