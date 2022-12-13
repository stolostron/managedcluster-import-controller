// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package source

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	workv1lister "open-cluster-management.io/api/client/work/listers/work/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

type InformerHolder struct {
	ImportSecretInformer cache.SharedIndexInformer
	ImportSecretLister   corev1listers.SecretLister

	AutoImportSecretInformer cache.SharedIndexInformer
	AutoImportSecretLister   corev1listers.SecretLister

	KlusterletWorkInformer cache.SharedIndexInformer
	KlusterletWorkLister   workv1lister.ManifestWorkLister

	HostedWorkInformer cache.SharedIndexInformer
	HostedWorkLister   workv1lister.ManifestWorkLister
}

// NewImportSecretSource return a source only for import secrets
func NewImportSecretSource(secretInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     secretInformer,
		expectedType: reflect.TypeOf(&corev1.Secret{}),
		name:         "import-secret",
	}
}

// NewAutoImportSecretSource return a source only for auto import secrets
func NewAutoImportSecretSource(secretInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     secretInformer,
		expectedType: reflect.TypeOf(&corev1.Secret{}),
		name:         "auto-import-secret",
	}
}

// NewKlusterletWorkSource return a source only for klusterlet manifest works
func NewKlusterletWorkSource(workInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     workInformer,
		expectedType: reflect.TypeOf(&workv1.ManifestWork{}),
		name:         "klusterlet-manifest-works",
	}
}

// NewHostedWorkSource return a source only for hosted manifest works
func NewHostedWorkSource(workInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     workInformer,
		expectedType: reflect.TypeOf(&workv1.ManifestWork{}),
		name:         "hosted-manifest-works",
	}
}

// Source is the event source of specified objects
type Source struct {
	informer     cache.SharedIndexInformer
	expectedType reflect.Type
	name         string
}

var _ source.SyncingSource = &Source{}

func (s *Source) Start(ctx context.Context, handler handler.EventHandler,
	queue workqueue.RateLimitingInterface, predicates ...predicate.Predicate) error {
	s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newObj, ok := obj.(client.Object)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", obj))
				return
			}

			if objType := reflect.TypeOf(newObj); s.expectedType != objType {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", obj))
				return
			}

			createEvent := event.CreateEvent{Object: newObj}

			for _, p := range predicates {
				if !p.Create(createEvent) {
					return
				}
			}

			handler.Create(createEvent, queue)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldClientObj, ok := oldObj.(client.Object)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", oldObj))
				return
			}

			if objType := reflect.TypeOf(oldClientObj); s.expectedType != objType {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", oldObj))
				return
			}

			newClientObj, ok := newObj.(client.Object)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", newObj))
				return
			}

			if objType := reflect.TypeOf(newClientObj); s.expectedType != objType {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", newObj))
				return
			}

			updateEvent := event.UpdateEvent{ObjectOld: oldClientObj, ObjectNew: newClientObj}

			for _, p := range predicates {
				if !p.Update(updateEvent) {
					return
				}
			}

			handler.Update(updateEvent, queue)
		},
		DeleteFunc: func(obj interface{}) {
			if _, ok := obj.(client.Object); !ok {
				// If the object doesn't have Metadata, assume it is a tombstone object of type DeletedFinalStateUnknown
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					klog.Errorf(fmt.Sprintf("Error decoding objects. Expected cache.DeletedFinalStateUnknown, type %T", obj))
					return
				}

				// Set obj to the tombstone obj
				obj = tombstone.Obj
			}

			o, ok := obj.(client.Object)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnDelete missing Object, type %T", obj))
				return
			}

			deleteEvent := event.DeleteEvent{Object: o}

			for _, p := range predicates {
				if !p.Delete(deleteEvent) {
					return
				}
			}

			handler.Delete(deleteEvent, queue)
		},
	})

	return nil
}

func (s *Source) WaitForSync(ctx context.Context) error {
	if ok := cache.WaitForCacheSync(ctx.Done(), s.informer.HasSynced); !ok {
		return fmt.Errorf("never achieved initial sync")
	}

	return nil
}

func (s *Source) String() string {
	return s.name
}

// Map a client object to reconcile request
type MapFunc func(client.Object) reconcile.Request

type ManagedClusterResourceEventHandler struct {
	MapFunc
}

var _ handler.EventHandler = &ManagedClusterResourceEventHandler{}

func (e *ManagedClusterResourceEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *ManagedClusterResourceEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectNew, q)
}

func (e *ManagedClusterResourceEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *ManagedClusterResourceEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// do nothing
}

func (e *ManagedClusterResourceEventHandler) add(obj client.Object, q workqueue.RateLimitingInterface) {
	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetNamespace(),
		},
	}
	if e.MapFunc != nil {
		request = e.MapFunc(obj)
	}
	q.Add(request)
}
