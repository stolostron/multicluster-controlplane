package clusterimport

import (
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/clusterimport/source"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"

	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	operatorv1listers "open-cluster-management.io/api/client/operator/listers/operator/v1"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "autoimport-controller"

func SetupCSRApprovalController(mgr manager.Manager, kubeClient kubernetes.Interface) error {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &CSRApprovalController{
			runtimeClient: mgr.GetClient(),
			kubeClient:    kubeClient,
		},
	})
	if err != nil {
		return err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &certificatesv1.CertificateSigningRequest{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return false },
		}),
	); err != nil {
		return err
	}
	return nil
}

func SetupClusterImportController(mgr manager.Manager,
	kubeClient kubernetes.Interface,
	klusterletClient operatorv1client.KlusterletInterface,
	secretInformer, klusterletInformer cache.SharedIndexInformer) error {

	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &ClusterImportController{
			kubeClient:       kubeClient,
			klusterletClient: klusterletClient,
			secretLister:     corev1listers.NewSecretLister(secretInformer.GetIndexer()),
			klusterletLister: operatorv1listers.NewKlusterletLister(klusterletInformer.GetIndexer()),
		},
	})
	if err != nil {
		return err
	}

	// watch the klusterlets
	if err := c.Watch(
		source.NewKlusterletSource(klusterletInformer),
		&source.ResourceEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc:  func(e event.UpdateEvent) bool { return true },
		}),
	); err != nil {
		return err
	}

	// watch the auto-import secrets
	if err := c.Watch(
		source.NewAutoImportSecretSource(secretInformer),
		&source.ResourceEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			DeleteFunc:  func(e event.DeleteEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				new, okNew := e.ObjectNew.(*corev1.Secret)
				old, okOld := e.ObjectOld.(*corev1.Secret)
				if okNew && okOld {
					return !equality.Semantic.DeepEqual(old.Data, new.Data)
				}
				return false
			},
		}),
	); err != nil {
		return err
	}

	return nil
}
