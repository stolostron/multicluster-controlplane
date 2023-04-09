package source

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"

	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// NewAutoImportSecretSource return a source only for auto import secrets
func NewAutoImportSecretSource(secretInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     secretInformer,
		expectedType: reflect.TypeOf(&corev1.Secret{}),
		name:         "auto-import-secret",
	}
}

func NewKlusterletSource(klusterletInformer cache.SharedIndexInformer) *Source {
	return &Source{
		informer:     klusterletInformer,
		expectedType: reflect.TypeOf(&operatorapiv1.Klusterlet{}),
		name:         "klusterlet",
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

type ResourceEventHandler struct {
	MapFunc
}

var _ handler.EventHandler = &ResourceEventHandler{}

func (e *ResourceEventHandler) Create(evt event.CreateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *ResourceEventHandler) Update(evt event.UpdateEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.ObjectNew, q)
}

func (e *ResourceEventHandler) Delete(evt event.DeleteEvent, q workqueue.RateLimitingInterface) {
	e.add(evt.Object, q)
}

func (e *ResourceEventHandler) Generic(evt event.GenericEvent, q workqueue.RateLimitingInterface) {
	// do nothing
}

func (e *ResourceEventHandler) add(obj client.Object, q workqueue.RateLimitingInterface) {
	clusterName := obj.GetNamespace()
	if len(clusterName) == 0 {
		clusterName = obj.GetName()
	}

	request := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: clusterName,
			Name:      clusterName,
		},
	}
	if e.MapFunc != nil {
		request = e.MapFunc(obj)
	}
	q.Add(request)
}
