// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package csr

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"reflect"
	"testing"

	"github.com/openshift/library-go/pkg/operator/events/eventstesting"
	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	clusterv1 "open-cluster-management.io/api/cluster/v1"

	certificatesv1 "k8s.io/api/certificates/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	csrNameReconcile = "csr-reconcile"
	clusterName      = "mycluster"
	agentName        = "klusterlet"
)

func newCSRRequest(t *testing.T, commonName string, orgs []string) []byte {
	t.Helper()
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	csrb, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: orgs,
		},
	}, pk)
	if err != nil {
		t.Fatalf("create csr: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrb})
}

func validClusterCSRSpec(t *testing.T, name, cluster, agent, username string) *certificatesv1.CertificateSigningRequest {
	t.Helper()
	return &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: cluster,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username:   username,
			SignerName: certificatesv1.KubeAPIServerClientSignerName,
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
			Request: newCSRRequest(t,
				helpers.SubjectPrefix+cluster+":"+agent,
				[]string{helpers.SubjectPrefix + cluster, helpers.ManagedClustersGroup},
			),
		},
	}
}

func TestReconcileCSR_Reconcile(t *testing.T) {
	testCSR := validClusterCSRSpec(t, csrNameReconcile, clusterName, agentName,
		fmt.Sprintf(userNameSignature, clusterName, clusterName))

	testSpecialClusterCSR := validClusterCSRSpec(t, csrNameReconcile, "specialCluster", agentName,
		fmt.Sprintf(userNameSignature, "specialCluster", "specialCluster"))

	maliciousCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username:   fmt.Sprintf(userNameSignature, clusterName, clusterName),
			SignerName: certificatesv1.KubeAPIServerClientSignerName,
			Usages:     []certificatesv1.KeyUsage{certificatesv1.UsageClientAuth},
			Request:    newCSRRequest(t, "system:admin", []string{"system:masters"}),
		},
	}

	testManagedCluster := &clusterv1.ManagedCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
		Spec: clusterv1.ManagedClusterSpec{},
	}

	testscheme := scheme.Scheme

	testscheme.AddKnownTypes(clusterv1.SchemeGroupVersion, &clusterv1.ManagedCluster{})

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name: csrNameReconcile,
		},
	}

	type fields struct {
		client     client.Client
		kubeClient *fakeclientset.Clientset
		scheme     *runtime.Scheme
	}
	type args struct {
		request reconcile.Request
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       reconcile.Result
		wantErr    bool
		wantApprove bool
	}{
		{
			name: "testCSR",
			fields: fields{
				client:     fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testManagedCluster, testCSR).Build(),
				kubeClient: fakeclientset.NewSimpleClientset(testCSR),
				scheme:     testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr:     false,
			wantApprove: true,
		},
		{
			name: "testCSRClusterNotFound",
			fields: fields{
				client:     fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testCSR).Build(),
				kubeClient: fakeclientset.NewSimpleClientset(testCSR.DeepCopy()),
				scheme:     testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr:     false,
			wantApprove: false,
		},
		{
			name: "testCSRSpecialCluster",
			fields: fields{
				client:     fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testSpecialClusterCSR).Build(),
				kubeClient: fakeclientset.NewSimpleClientset(testSpecialClusterCSR),
				scheme:     testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr:     false,
			wantApprove: true,
		},
		{
			name: "rejectsMaliciousSubject",
			fields: fields{
				client:     fake.NewClientBuilder().WithScheme(testscheme).WithObjects(testManagedCluster, maliciousCSR).Build(),
				kubeClient: fakeclientset.NewSimpleClientset(maliciousCSR),
				scheme:     testscheme,
			},
			args: args{
				request: req,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr:     false,
			wantApprove: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test name: %s", tt.name)
			clientHolder := &helpers.ClientHolder{
				KubeClient:    tt.fields.kubeClient,
				RuntimeClient: tt.fields.client,
			}
			r := &ReconcileCSR{
				clientHolder: clientHolder,
				recorder:     eventstesting.NewTestingEventRecorder(t),
				approvalConditions: []func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error){
					func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error) {
						clusterName := helpers.GetClusterName(csr)
						cluster := clusterv1.ManagedCluster{}
						err := clientHolder.RuntimeClient.Get(ctx, types.NamespacedName{Name: clusterName}, &cluster)
						if errors.IsNotFound(err) {
							return false, nil
						}
						if err != nil {
							return false, err
						}
						return true, nil
					},
					func(ctx context.Context, csr *certificatesv1.CertificateSigningRequest) (bool, error) {
						clusterName := helpers.GetClusterName(csr)
						if clusterName == "specialCluster" {
							return true, nil
						}
						return false, nil
					},
				},
			}
			got, err := r.Reconcile(context.TODO(), tt.args.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileCSR.Reconcile() Creation error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileCSR.Reconcile() Creation= %v, want %v", got, tt.want)
			}
			if !tt.wantErr && !got.Requeue {
				csr, err := r.clientHolder.KubeClient.CertificatesV1().CertificateSigningRequests().Get(
					context.TODO(), csrNameReconcile, metav1.GetOptions{})
				if err != nil {
					t.Error("CSR not found")
				}
				approved := false
				for _, c := range csr.Status.Conditions {
					if c.Type == certificatesv1.CertificateApproved {
						approved = true
						break
					}
				}
				if approved != tt.wantApprove {
					t.Errorf("approved = %v, wantApprove %v", approved, tt.wantApprove)
				}
			}
		})
	}

}

func Test_getApproval(t *testing.T) {
	testCSRNoApproval := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
	}

	testCSRApproved := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{Type: certificatesv1.CertificateApproved},
			},
		},
	}

	testCSRDenied := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
		Status: certificatesv1.CertificateSigningRequestStatus{
			Conditions: []certificatesv1.CertificateSigningRequestCondition{
				{Type: certificatesv1.CertificateDenied},
			},
		},
	}

	type args struct {
		csr *certificatesv1.CertificateSigningRequest
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "testCSRNoApproval",
			args: args{
				csr: testCSRNoApproval,
			},
			want: "",
		},
		{
			name: "testCSRApproved",
			args: args{
				csr: testCSRApproved,
			},
			want: string(certificatesv1.CertificateApproved),
		},
		{
			name: "testCSRDenied",
			args: args{
				csr: testCSRDenied,
			},
			want: string(certificatesv1.CertificateDenied),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getApprovalType(tt.args.csr); got != tt.want {
				t.Errorf("getApprovalType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_validUsername(t *testing.T) {
	testCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
	}

	testCSRBadUsername := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: "badUserName",
		},
	}

	type args struct {
		csr         *certificatesv1.CertificateSigningRequest
		clusterName string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "testCSR",
			args: args{
				csr:         testCSR,
				clusterName: clusterName,
			},
			want: true,
		},
		{
			name: "testCSRBadUsername",
			args: args{
				csr:         testCSRBadUsername,
				clusterName: clusterName,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validUsername(tt.args.csr, tt.args.clusterName); got != tt.want {
				t.Errorf("validUsername() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_isValidUnapprovedBootstrapCSR(t *testing.T) {
	testCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				constants.CSRClusterNameLabel: clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
	}

	testCSRBadLabel := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrNameReconcile,
			Labels: map[string]string{
				"badLabel": clusterName,
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Username: fmt.Sprintf(userNameSignature, clusterName, clusterName),
		},
	}

	type args struct {
		csr *certificatesv1.CertificateSigningRequest
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "testCSR",
			args: args{
				csr: testCSR,
			},
			want: true,
		},
		{
			name: "testCSRBadLabel",
			args: args{
				csr: testCSRBadLabel,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidUnapprovedBootstrapCSR(tt.args.csr); got != tt.want {
				t.Errorf("csrPredicate() = %v, want %v", got, tt.want)
			}
		})
	}
}
