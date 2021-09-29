// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package source

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var _ source.SyncingSource = &SecretSource{}

// SecretSource is the event source of specified secrets
type SecretSource struct {
	secretInformer cache.SharedIndexInformer
}

// NewImportSecretSource return a SecretSource only for import secrets
func NewImportSecretSource(secretInformer cache.SharedIndexInformer) *SecretSource {
	return &SecretSource{secretInformer: secretInformer}
}

// NewAutoImportSecretSource return a SecretSource only for auto import secrets
func NewAutoImportSecretSource(secretInformer cache.SharedIndexInformer) *SecretSource {
	return &SecretSource{secretInformer: secretInformer}
}

func (s *SecretSource) Start(ctx context.Context, handler handler.EventHandler, queue workqueue.RateLimitingInterface,
	predicates ...predicate.Predicate) error {
	s.secretInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			newSecret, ok := obj.(*corev1.Secret)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnAdd missing Object, type %T", obj))
				return
			}

			createEvent := event.CreateEvent{Object: newSecret}

			for _, p := range predicates {
				if !p.Create(createEvent) {
					return
				}
			}

			handler.Create(createEvent, queue)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			oldSecret, ok := oldObj.(*corev1.Secret)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnUpdate missing ObjectOld, type %T", oldObj))
				return
			}

			newSecret, ok := newObj.(*corev1.Secret)
			if !ok {
				klog.Errorf(fmt.Sprintf("OnUpdate missing ObjectNew, type %T", newObj))
				return
			}

			updateEvent := event.UpdateEvent{ObjectOld: oldSecret, ObjectNew: newSecret}

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

func (s *SecretSource) WaitForSync(ctx context.Context) error {
	if ok := cache.WaitForCacheSync(ctx.Done(), s.secretInformer.HasSynced); !ok {
		return fmt.Errorf("never achieved initial sync")
	}

	return nil
}

type ManagedClusterSecretEventHandler struct{}

var _ handler.EventHandler = &ManagedClusterSecretEventHandler{}

func (e *ManagedClusterSecretEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	// the secret namespace is the manged cluster name, we only send create request with secret namespace
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: evt.Object.GetNamespace(), Name: evt.Object.GetNamespace()}})
}

func (e *ManagedClusterSecretEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	// the secret namespace is the manged cluster name, we only send update request with secret namespace
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: evt.ObjectNew.GetNamespace(), Name: evt.ObjectNew.GetNamespace()}})
}

func (e *ManagedClusterSecretEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	// the secret namespace is the manged cluster name, we only send delet request with secret namespace
	q.Add(reconcile.Request{NamespacedName: types.NamespacedName{Namespace: evt.Object.GetNamespace(), Name: evt.Object.GetNamespace()}})
}

func (e *ManagedClusterSecretEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// do nothing
}
