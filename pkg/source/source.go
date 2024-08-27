// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package source

import (
	"context"
	"fmt"
	"reflect"

	klusterletconfigv1alpha1lister "github.com/stolostron/cluster-lifecycle-api/client/klusterletconfig/listers/klusterletconfig/v1alpha1"
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

	KlusterletConfigInformer cache.SharedIndexInformer
	KlusterletConfigLister   klusterletconfigv1alpha1lister.KlusterletConfigLister

	ManagedClusterInformer cache.SharedIndexInformer
}

// NewImportSecretSource return a source only for import secrets
func NewImportSecretSource(secretInformer cache.SharedIndexInformer,
	handler handler.EventHandler,
	predicates ...predicate.Predicate) *Source {
	return &Source{
		informer:     secretInformer,
		expectedType: reflect.TypeOf(&corev1.Secret{}),
		name:         "import-secret",

		handler:    handler,
		predicates: predicates,
	}
}

// NewAutoImportSecretSource return a source only for auto import secrets
func NewAutoImportSecretSource(secretInformer cache.SharedIndexInformer,
	handler handler.EventHandler,
	predicates ...predicate.Predicate) *Source {
	return &Source{
		informer:     secretInformer,
		expectedType: reflect.TypeOf(&corev1.Secret{}),
		name:         "auto-import-secret",

		handler:    handler,
		predicates: predicates,
	}
}

// NewKlusterletWorkSource return a source only for klusterlet manifest works
func NewKlusterletWorkSource(workInformer cache.SharedIndexInformer,
	handler handler.EventHandler,
	predicates ...predicate.Predicate) *Source {
	return &Source{
		informer:     workInformer,
		expectedType: reflect.TypeOf(&workv1.ManifestWork{}),
		name:         "klusterlet-manifest-works",

		handler:    handler,
		predicates: predicates,
	}
}

// NewHostedWorkSource return a source only for hosted manifest works
func NewHostedWorkSource(workInformer cache.SharedIndexInformer,
	handler handler.EventHandler,
	predicates ...predicate.Predicate) *Source {
	return &Source{
		informer:     workInformer,
		expectedType: reflect.TypeOf(&workv1.ManifestWork{}),
		name:         "hosted-manifest-works",

		handler:    handler,
		predicates: predicates,
	}
}

// Source is the event source of specified objects
type Source struct {
	informer     cache.SharedIndexInformer
	expectedType reflect.Type
	name         string

	handler    handler.EventHandler
	predicates []predicate.Predicate
}

var _ source.SyncingSource = &Source{}

func (s *Source) Start(ctx context.Context, queue workqueue.TypedRateLimitingInterface[reconcile.Request]) error {
	_, err := s.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
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

			for _, p := range s.predicates {
				if !p.Create(createEvent) {
					return
				}
			}

			s.handler.Create(ctx, createEvent, queue)
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

			for _, p := range s.predicates {
				if !p.Update(updateEvent) {
					return
				}
			}

			s.handler.Update(ctx, updateEvent, queue)
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

			for _, p := range s.predicates {
				if !p.Delete(deleteEvent) {
					return
				}
			}

			s.handler.Delete(ctx, deleteEvent, queue)
		},
	})

	return err
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

func (e *ManagedClusterResourceEventHandler) Create(ctx context.Context, evt event.TypedCreateEvent[client.Object],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.add(evt.Object, q)
}

func (e *ManagedClusterResourceEventHandler) Update(ctx context.Context, evt event.TypedUpdateEvent[client.Object],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.add(evt.ObjectNew, q)
}

func (e *ManagedClusterResourceEventHandler) Delete(ctx context.Context, evt event.TypedDeleteEvent[client.Object],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	e.add(evt.Object, q)
}

func (e *ManagedClusterResourceEventHandler) Generic(ctx context.Context, evt event.TypedGenericEvent[client.Object],
	q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
	// do nothing
}

func (e *ManagedClusterResourceEventHandler) add(obj client.Object, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
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
