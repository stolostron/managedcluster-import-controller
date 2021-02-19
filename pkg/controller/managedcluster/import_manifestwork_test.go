//Package managedcluster ...
package managedcluster

import (
	"context"
	"os"
	"reflect"
	"testing"

	clusterv1 "github.com/open-cluster-management/api/cluster/v1"
	workv1 "github.com/open-cluster-management/api/work/v1"
	ocinfrav1 "github.com/openshift/api/config/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	imagePullSecretNameMW = "my-image-pul-secret-mw"
	managedClusterNameMW  = "cluster-mw"
)

func init() {
	os.Setenv(registrationImageEnvVarName, "quay.io/open-cluster-management/registration:latest")
}

func Test_manifestWorktNsN(t *testing.T) {
	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "testmanagedcluster",
		},
	}

	type args struct {
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		want    types.NamespacedName
		wantErr bool
	}{
		{
			name: "nil managedCluster",
			args: args{
				managedCluster: nil,
			},
			want:    types.NamespacedName{},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				managedCluster: testManagedCluster,
			},
			want: types.NamespacedName{
				Name:      "testmanagedcluster" + manifestWorkNamePostfix,
				Namespace: "testmanagedcluster",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			got, err := manifestWorkNsN(tt.args.managedCluster)
			if (err != nil) != tt.wantErr {
				t.Errorf("bootstrapServiceAccountNsN() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("bootstrapServiceAccountNsN() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newManifestWorks(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "newmanifestwork",
		},
	}

	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testscheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "newmanifestwork" + bootstrapServiceAccountNamePostfix,
			Namespace: "newmanifestwork",
		},
	}

	tokenSecret, err := serviceAccountTokenSecret(testSA)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	testSA.Secrets = append(testSA.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	testClient := fake.NewFakeClientWithScheme(testscheme, []runtime.Object{
		testSA, tokenSecret, testInfraConfig, imagePullSecret,
	}...)

	type args struct {
		managedCluster *clusterv1.ManagedCluster
	}
	type manifestworks struct {
		crds  *workv1.ManifestWork
		yamls *workv1.ManifestWork
	}
	tests := []struct {
		name    string
		args    args
		want    manifestworks
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				managedCluster: nil,
			},
			want: manifestworks{
				crds:  nil,
				yamls: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				managedCluster: testManagedCluster,
			},
			want: manifestworks{
				crds: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "newmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "newmanifestwork",
					},
				},
				yamls: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "newmanifestwork" + manifestWorkNamePostfix,
						Namespace: "newmanifestwork",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			crds, yamls, err := generateImportYAMLs(testClient, tt.args.managedCluster, []string{})
			if (err != nil) != tt.wantErr {
				t.Errorf("generateImportYAMLs error=%v, wantErr %v", err, tt.wantErr)
			}
			gotCRDs, gotYAMLs, err := newManifestWorks(testClient, tt.args.managedCluster, crds, yamls)
			if (err != nil) != tt.wantErr {
				t.Errorf("newManifestWork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want.crds == nil {
				if gotCRDs != nil {
					t.Errorf("newManifestWorks() = %v, want %v", gotCRDs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.crds.GetNamespace() || gotCRDs.GetName() != tt.want.crds.GetName() {
					t.Errorf("newManifestWorks() = %v, want %v", gotCRDs, tt.want.crds)
				}
			}
			if tt.want.yamls == nil {
				if gotYAMLs != nil {
					t.Errorf("newManifestWorks() = %v, want %v", gotYAMLs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.yamls.GetNamespace() || gotYAMLs.GetName() != tt.want.yamls.GetName() {
					t.Errorf("newManifestWorks() = %v, want %v", gotYAMLs, tt.want.yamls)
				}
			}
		})
	}

}

func Test_createOrUpdateManifestWork(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "createmanifestwork",
		},
	}

	testScheme := scheme.Scheme

	testScheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testScheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{}, &ocinfrav1.APIServer{})

	testSA := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createmanifestwork" + bootstrapServiceAccountNamePostfix,
			Namespace: "createmanifestwork",
		},
	}

	tokenSecret, err := serviceAccountTokenSecret(testSA)
	if err != nil {
		t.Errorf("fail to initialize serviceaccount token secret, error = %v", err)
	}

	testSA.Secrets = append(testSA.Secrets, corev1.ObjectReference{
		Name: tokenSecret.Name,
	})

	crds := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
			Namespace: "createmanifestwork",
		},
	}
	yamls := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createmanifestwork" + manifestWorkNamePostfix,
			Namespace: "createmanifestwork",
		},
	}

	crdsUpdate := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
			Namespace: "createmanifestwork",
		},
		Status: workv1.ManifestWorkStatus{
			Conditions: []workv1.StatusCondition{
				{Type: string(workv1.WorkApplied)},
			},
		},
	}
	yamlsUpdate := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "createmanifestwork" + manifestWorkNamePostfix,
			Namespace: "createmanifestwork",
		},
		Status: workv1.ManifestWorkStatus{
			Conditions: []workv1.StatusCondition{
				{Type: string(workv1.WorkApplied)},
			},
		},
	}
	type args struct {
		client         client.Client
		managedCluster *clusterv1.ManagedCluster
	}

	type manifestworks struct {
		crds  *workv1.ManifestWork
		yamls *workv1.ManifestWork
	}
	tests := []struct {
		name    string
		args    args
		want    manifestworks
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: nil,
			},
			want: manifestworks{
				crds:  nil,
				yamls: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: manifestworks{
				crds: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "createmanifestwork",
					},
				},
				yamls: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix,
						Namespace: "createmanifestwork",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success no change",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, crds, yamls, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: manifestworks{
				crds: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "createmanifestwork",
					},
				},
				yamls: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix,
						Namespace: "createmanifestwork",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success with change",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					testSA, tokenSecret, testInfraConfig, crdsUpdate, yamlsUpdate, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			want: manifestworks{
				crds: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "createmanifestwork",
					},
				},
				yamls: &workv1.ManifestWork{
					TypeMeta: metav1.TypeMeta{
						APIVersion: workv1.SchemeGroupVersion.String(),
						Kind:       "ManifestWork",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "createmanifestwork" + manifestWorkNamePostfix,
						Namespace: "createmanifestwork",
					},
				},
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			crds, yamls, err := generateImportYAMLs(tt.args.client, tt.args.managedCluster, []string{})
			if (err != nil) != tt.wantErr {
				t.Errorf("generateImportYAMLs error=%v, wantErr %v", err, tt.wantErr)
			}
			gotCRDs, gotYAMLs, err := createOrUpdateManifestWorks(tt.args.client, testScheme, tt.args.managedCluster, crds, yamls)
			if (err != nil) != tt.wantErr {
				t.Errorf("createManifestWork() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want.crds == nil {
				if gotCRDs != nil {
					t.Errorf("createManifestWorks() = %v, want %v", gotCRDs, tt.want)
				}
			} else {
				if gotCRDs.GetNamespace() != tt.want.crds.GetNamespace() || gotCRDs.GetName() != tt.want.crds.GetName() {
					t.Errorf("createManifestWorks() = %v, want %v", gotCRDs, tt.want.crds)
				}
				if len(gotCRDs.Status.Conditions) != 0 {
					t.Error("createManifestWorks() crds not updated")
				}
			}
			if tt.want.yamls == nil {
				if gotYAMLs != nil {
					t.Errorf("createManifestWorks() = %v, want %v", gotYAMLs, tt.want)
				}
			} else {
				if gotYAMLs.GetNamespace() != tt.want.yamls.GetNamespace() || gotYAMLs.GetName() != tt.want.yamls.GetName() {
					t.Errorf("createManifestWorks() = %v, want %v", gotYAMLs, tt.want.yamls)
				}
				if len(gotYAMLs.Status.Conditions) != 0 {
					t.Error("createManifestWorks() yamls not updated")
				}
			}
		})
	}
}

func Test_deleteManifestWorks(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	testInfraConfig := &ocinfrav1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Spec: ocinfrav1.InfrastructureSpec{},
		Status: ocinfrav1.InfrastructureStatus{
			APIServerURL: "http://127.0.0.1:6443",
		},
	}

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "deletemanifestwork",
		},
	}
	testScheme := scheme.Scheme

	testScheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})
	testScheme.AddKnownTypes(ocinfrav1.SchemeGroupVersion, &ocinfrav1.Infrastructure{})

	crds := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deletemanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
			Namespace: "deletemanifestwork",
		},
	}
	yamls := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "deletemanifestwork" + manifestWorkNamePostfix,
			Namespace: "deletemanifestwork",
		},
	}

	type args struct {
		client         client.Client
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls, testInfraConfig, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			if err := deleteKlusterletManifestWorks(tt.args.client, tt.args.managedCluster); (err != nil) != tt.wantErr {
				t.Errorf("deleteManifestWorks() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				crds := &workv1.ManifestWork{}
				err := tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "deletemanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "deletemanifestwork",
					}, crds)
				if err == nil {
					t.Error("deleteManifestWorks crds manifest not deleted")
				}
				yamls := &workv1.ManifestWork{}
				err = tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "deletemanifestwork" + manifestWorkNamePostfix,
						Namespace: "deletemanifestwork",
					}, yamls)
				if err == nil {
					t.Error("deleteManifestWorks yamls manifest not deleted")
				}
			}
		})
	}
}

func Test_evictManifestWorks(t *testing.T) {
	os.Setenv("DEFAULT_IMAGE_PULL_SECRET", imagePullSecretNameSecret)
	os.Setenv("POD_NAMESPACE", managedClusterNameSecret)
	imagePullSecret := newFakeImagePullSecret()

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "evictmanifestwork",
		},
	}
	testScheme := scheme.Scheme

	testScheme.AddKnownTypes(workv1.SchemeGroupVersion, &workv1.ManifestWork{})
	testScheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})

	crds := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "evictmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
			Namespace:  "evictmanifestwork",
			Finalizers: []string{"evict-finalizer"},
		},
	}
	yamls := &workv1.ManifestWork{
		TypeMeta: metav1.TypeMeta{
			APIVersion: workv1.SchemeGroupVersion.String(),
			Kind:       "ManifestWork",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       "evictmanifestwork" + manifestWorkNamePostfix,
			Namespace:  "evictmanifestwork",
			Finalizers: []string{"evict-finalizer"},
		},
	}

	type args struct {
		client         client.Client
		managedCluster *clusterv1.ManagedCluster
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "nil cluster",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls, imagePullSecret,
				}...),
				managedCluster: nil,
			},
			wantErr: true,
		},
		{
			name: "success",
			args: args{
				client: fake.NewFakeClientWithScheme(testScheme, []runtime.Object{
					crds, yamls, imagePullSecret,
				}...),
				managedCluster: testManagedCluster,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := evictKlusterletManifestWorks(tt.args.client, tt.args.managedCluster); (err != nil) != tt.wantErr {
				t.Errorf("evictManifestWorks() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				crdsGet := &workv1.ManifestWork{}
				err := tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "evictmanifestwork" + manifestWorkNamePostfix + manifestWorkCRDSPostfix,
						Namespace: "evictmanifestwork",
					}, crdsGet)
				if err != nil {
					t.Error("deleteManifestWorks crds manifest deleted")
				}
				if len(crdsGet.Finalizers) > 0 {
					t.Error("CRDs finalizers not removed")
				}
				yamlsGet := &workv1.ManifestWork{}
				err = tt.args.client.Get(context.TODO(),
					types.NamespacedName{
						Name:      "evictmanifestwork" + manifestWorkNamePostfix,
						Namespace: "evictmanifestwork",
					}, yamlsGet)
				if err != nil {
					t.Error("deleteManifestWorks yamls manifest deleted")
				}
				if len(yamlsGet.Finalizers) > 0 {
					t.Error("YAMLs finalizers not removed")
				}
			}
		})
	}

}
