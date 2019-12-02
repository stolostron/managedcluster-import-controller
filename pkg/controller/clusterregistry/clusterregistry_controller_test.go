//Package clusterregistry contains common utility functions that gets call by many differerent packages
// Copyright 2019 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package clusterregistry

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	mcmv1alpha1 "github.ibm.com/IBMPrivateCloud/hcm-api/pkg/apis/mcm/v1alpha1"
	"github.ibm.com/IBMPrivateCloud/mcm-cluster-controller/pkg/clusterimport"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	clusterregistryv1alpha1 "k8s.io/cluster-registry/pkg/apis/clusterregistry/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestInstanceDNE(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)

	objs := []runtime.Object{cluster}
	cl := fake.NewFakeClient(objs...)

	r := &ReconcileCluster{client: cl, scheme: s}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	//cluster resource is deleted before the reconcile starts
	err := cl.Delete(context.TODO(), cluster)
	assert.NoError(t, err)

	res, err := r.Reconcile(req)

	assert.NoError(t, err, "Reconcile have no error")
	assert.False(t, res.Requeue, "Requeue is false since instance no longer exist")
}

func TestNewClusterCreate(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)

	objs := []runtime.Object{cluster}
	cl := fake.NewFakeClient(objs...)

	r := &ReconcileCluster{client: cl, scheme: s}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(req)

	assert.NoError(t, err, "Reconcile have no error")
	assert.False(t, res.Requeue, "Requeue false")

	err = cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, cluster)

	assert.NoError(t, err, "ClusterRegistry Cluster exist after reconcile")

	finalizerExist := false
	finalizerUnique := true

	for _, finalizer := range cluster.Finalizers {
		if finalizer == ClusterControllerFinalizer {
			if finalizerExist == true {
				finalizerUnique = false
			}

			finalizerExist = true
		}
	}

	assert.True(t, finalizerExist, "Finalizer exist")
	assert.True(t, finalizerUnique, "Finalizer unique")
}

func TestIsDeleting(t *testing.T) {
	notDeletingCluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "test-cluster",
		},
		Status: clusterregistryv1alpha1.ClusterStatus{}, //pending cluster have empty status
	}
	assert.False(t, isDeleting(notDeletingCluster))

	deletingCluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cluster",
			Namespace:         "test-cluster",
			DeletionTimestamp: &metav1.Time{},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{}, //pending cluster have empty status
	}
	assert.True(t, isDeleting(deletingCluster))
}

func TestIsOffline(t *testing.T) {
	pendingCluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cluster",
			Namespace:         "test-cluster",
			DeletionTimestamp: &metav1.Time{},
			Finalizers:        []string{ClusterControllerFinalizer},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{}, //pending cluster have empty status
	}
	assert.True(t, isOffline(pendingCluster))

	offlineCluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cluster",
			Namespace:         "test-cluster",
			DeletionTimestamp: &metav1.Time{},
			Finalizers:        []string{ClusterControllerFinalizer},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: "",
				},
			},
		},
	}
	assert.True(t, isOffline(offlineCluster))

	onlineCluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-cluster",
			Namespace:         "test-cluster",
			DeletionTimestamp: &metav1.Time{},
			Finalizers:        []string{ClusterControllerFinalizer},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: clusterregistryv1alpha1.ClusterOK,
				},
			},
		},
	}
	assert.False(t, isOffline(onlineCluster))
}

func TestPendingClusterDelete(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	now := metav1.Now()

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			DeletionTimestamp: &now,
			Finalizers: []string{
				ClusterControllerFinalizer,
				PlatformAPIFinalizer,
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{}, //pending cluster have empty status
	}
	assert.True(t, isDeleting(cluster))

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)

	objs := []runtime.Object{cluster}
	cl := fake.NewFakeClient(objs...)

	r := &ReconcileCluster{client: cl, scheme: s}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}

	res, err := r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.False(t, res.Requeue, "Cluster being deleted no need to requeue")

	cluster = &clusterregistryv1alpha1.Cluster{}
	err = cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, cluster)
	assert.NoError(t, err)
	assert.Empty(t, cluster.Finalizers, "Finalizer does not exist")
}

func TestOnlineClusterDelete(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	now := metav1.Now()

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			DeletionTimestamp: &now,
			Finalizers: []string{
				ClusterControllerFinalizer,
				PlatformAPIFinalizer,
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: clusterregistryv1alpha1.ClusterOK,
				},
			},
		},
	}
	assert.True(t, isDeleting(cluster))

	work := &mcmv1alpha1.Work{}

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)
	s.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, work)

	objs := []runtime.Object{cluster}
	cl := fake.NewFakeClient(objs...)

	r := &ReconcileCluster{client: cl, scheme: s}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.True(t, res.Requeue, "Cluster in the process of being deleted requeue till its offline")
	assert.Equal(t, res.RequeueAfter, 5*time.Second, "Requeue in 5 second")

	err = cl.Get(context.TODO(), types.NamespacedName{Name: "delete-multicluster-endpoint", Namespace: namespace}, work)
	assert.NoError(t, err, "self destruct work exist")

	//wait for self destruct to complete
	res, err = r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.True(t, res.Requeue, "Cluster in the process of being deleted requeue till its offline")
	assert.Equal(t, res.RequeueAfter, 5*time.Second, "Requeue in 5 second")

	//self destruct completed, cluster goes offline
	cluster.Status.Conditions[0].Type = ""
	err = cl.Update(context.TODO(), cluster)
	assert.NoError(t, err)

	res, err = r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.False(t, res.Requeue, "Cluster being deleted no need to requeue")

	cluster = &clusterregistryv1alpha1.Cluster{}
	err = cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, cluster)
	assert.NoError(t, err)
	assert.Empty(t, cluster.Finalizers, "Finalizer does not exist")
}

func TestOnlineClusterDeleteWithSecret(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	now := metav1.Now()

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			DeletionTimestamp: &now,
			Finalizers: []string{
				ClusterControllerFinalizer,
				PlatformAPIFinalizer,
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: clusterregistryv1alpha1.ClusterOK,
				},
			},
		},
	}
	assert.True(t, isDeleting(cluster))

	clusterConfig := []byte(`
clusterLabels:
  cloud: auto-detect
  vendor: auto-detect
version: latest
applicationManager:
  enabled: true
tillerIntegration:
  enabled: true
prometheusIntegration:
  enabled: true
topologyCollector:
  enabled: true
updateInterval: 15
searchCollector:
  enabled: true
policyController:
  enabled: true
serviceRegistry:
  enabled: true
  dnsSuffix: mcm.svc
  plugins: kube-service
metering:
  enabled: false
private_registry_enabled: true
docker_username: user@company.com
docker_password: user_password
imageRegistry: registry.com/project
imageNamePostfix: -amd64
clusterName: test-cluster
clusterNamespace: test-cluster
`)

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cluster.Name + "-secret",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"config.yaml": clusterConfig,
		},
	}

	work := &mcmv1alpha1.Work{}
	job := &batchv1.Job{}

	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)
	s.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, work)
	s.AddKnownTypes(corev1.SchemeGroupVersion, secret)
	s.AddKnownTypes(batchv1.SchemeGroupVersion, job)

	objs := []runtime.Object{cluster, secret}
	cl := fake.NewFakeClient(objs...)

	//test getClusterSecret
	foundClusterSecret := getClusterSecret(cl, cluster)
	assert.NotNil(t, foundClusterSecret)
	assert.Equal(t, foundClusterSecret.Name, secret.Name)
	assert.Equal(t, foundClusterSecret.Namespace, secret.Namespace)
	assert.Equal(t, foundClusterSecret.Data, secret.Data)

	r := &ReconcileCluster{client: cl, scheme: s}

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		},
	}
	res, err := r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.True(t, res.Requeue, "Cluster in the process of being deleted requeue till its offline")
	assert.Equal(t, res.RequeueAfter, 5*time.Second, "Requeue in 5 second")

	//test getSelfDestructWork
	work = getSelfDestructWork(cl, cluster)
	assert.NotNil(t, work)
	assert.Equal(t, work.Name, selfDestructWorkName)
	assert.NotNil(t, work.Spec.KubeWork)

	expectedSelfDestructImage := fmt.Sprintf("%s/%s%s:%s", "registry.com/project", clusterimport.DefaultOperatorImage, "-amd64", "latest")

	err = json.Unmarshal(work.Spec.KubeWork.ObjectTemplate.Raw, job)
	assert.NoError(t, err)
	assert.Equal(t, job.Name, selfDestructJobName)
	assert.Equal(t, job.Namespace, "multicluster-endpoint")
	assert.NotEmpty(t, job.Spec.Template.Spec.ImagePullSecrets)
	assert.Contains(t, job.Spec.Template.Spec.ImagePullSecrets, corev1.LocalObjectReference{Name: clusterimport.DefaultImagePullSecretName})
	assert.Equal(t, job.Spec.Template.Spec.Containers[0].Image, expectedSelfDestructImage)

	//wait for self destruct to complete
	res, err = r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.True(t, res.Requeue, "Cluster in the process of being deleted requeue till its offline")
	assert.Equal(t, res.RequeueAfter, 5*time.Second, "Requeue in 5 second")

	//self destruct completed, cluster goes offline
	cluster.Status.Conditions[0].Type = ""
	err = cl.Update(context.TODO(), cluster)
	assert.NoError(t, err)

	res, err = r.Reconcile(req)
	assert.NoError(t, err, "Reconcile have no error")
	assert.False(t, res.Requeue, "Cluster being deleted no need to requeue")

	cluster = &clusterregistryv1alpha1.Cluster{}
	err = cl.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: namespace}, cluster)
	assert.NoError(t, err)
	assert.Empty(t, cluster.Finalizers, "Finalizer does not exist")
}

func TestGetSelfDestructWork(t *testing.T) {
	var (
		name      = "test-cluster"
		namespace = "test-cluster"
	)

	cluster := &clusterregistryv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Finalizers: []string{
				ClusterControllerFinalizer,
				PlatformAPIFinalizer,
			},
		},
		Status: clusterregistryv1alpha1.ClusterStatus{
			Conditions: []clusterregistryv1alpha1.ClusterCondition{
				{
					Type: clusterregistryv1alpha1.ClusterOK,
				},
			},
		},
	}

	work := &mcmv1alpha1.Work{}
	s := scheme.Scheme
	s.AddKnownTypes(clusterregistryv1alpha1.SchemeGroupVersion, cluster)
	s.AddKnownTypes(mcmv1alpha1.SchemeGroupVersion, work)

	randomWork := &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "not-self-destruct",
			Namespace: namespace,
		},
	}

	selfDestructWork := &mcmv1alpha1.Work{
		ObjectMeta: metav1.ObjectMeta{
			Name:      selfDestructWorkName,
			Namespace: namespace,
		},
	}

	objs := []runtime.Object{randomWork, cluster}

	cl := fake.NewFakeClient(objs...)

	outputWork := getSelfDestructWork(cl, cluster)
	assert.Nil(t, outputWork)

	err := cl.Create(context.TODO(), selfDestructWork)
	assert.NoError(t, err)

	outputWork = getSelfDestructWork(cl, cluster)
	assert.NotNil(t, outputWork)
	assert.Equal(t, outputWork.Name, selfDestructWorkName)
}
