package controller

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	hyperv1beta1 "github.com/openshift/hypershift/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
)

const (
	// hostedClusterImportAnnotation indicates whether to import hostedcluster as managed cluster
	hostedClusterImportAnnotation = "cluster.open-cluster-management.io/import-managedcluster"
	// hostedClusterImportFinalizer is the finalizer added to hostedcluster for importing as managed cluster
	hostedClusterImportFinalizer = "cluster.open-cluster-management.io/import-managedcluster-finalizer"
)

// HostedClusterReconciler reconciles a HostedCluster object
type HostedClusterReconciler struct {
	client.Client
	// Scheme *runtime.Scheme
	ControlplaneKubeClient     kubernetes.Interface
	ControlplaneOperatorClient operatorclient.Interface
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *HostedClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(4).Info("Reconciling hostecluster", "namespace", req.NamespacedName.Namespace, "name", req.NamespacedName.Name)

	hostedCluster := &hyperv1beta1.HostedCluster{}
	if err := r.Get(ctx, req.NamespacedName, hostedCluster); err != nil {
		log.Error(err, "unable to fetch hostedcluster")
		// we'll ignore not-found errors, since they can't be fixed by an immediate
		// requeue (we'll need to wait for a new notification), and we can get them
		// on deleted requests.
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// hostedcluster is terminating, detach the managed cluster and return
	if !hostedCluster.GetDeletionTimestamp().IsZero() {
		if containStr(hostedCluster.GetFinalizers(), hostedClusterImportFinalizer) {
			if err := r.pruneManagedClusterResources(ctx, log, hostedCluster); err != nil {
				return ctrl.Result{}, err
			}
			hostedCluster.SetFinalizers(removeStr(hostedCluster.GetFinalizers(), hostedClusterImportFinalizer))
			if err := r.Client.Update(ctx, hostedCluster, &client.UpdateOptions{}); err != nil {
				if errors.IsConflict(err) {
					log.V(4).Info("conflict when removing finalizer from hostedcluster instance", "hostedcluster", req.NamespacedName)
					return ctrl.Result{Requeue: true}, nil
				} else if err != nil {
					log.Error(err, "unable to remove finalizer to hostedcluster instance", "hostedcluster", req.NamespacedName)
					return ctrl.Result{}, err
				}
			}
		}

		return ctrl.Result{}, nil
	}

	// check klusterlet exist, if it already exists, then do nothing
	if _, err := r.ControlplaneOperatorClient.OperatorV1().Klusterlets().Get(ctx, req.NamespacedName.Name, metav1.GetOptions{}); err != nil {
		return ctrl.Result{}, nil
	}

	if !containStr(hostedCluster.GetFinalizers(), hostedClusterImportFinalizer) {
		hostedCluster.SetFinalizers(append(hostedCluster.GetFinalizers(), hostedClusterImportFinalizer))
		if err := r.Client.Update(ctx, hostedCluster, &client.UpdateOptions{}); err != nil {
			if errors.IsConflict(err) {
				log.Info("conflict when adding finalizer to hostedcluster instance", "hostedcluster", req.NamespacedName)
				return ctrl.Result{Requeue: true}, nil
			} else if err != nil {
				log.Error(err, "unable to add finalizer to hostedcluster instance", "hostedcluster", req.NamespacedName)
				return ctrl.Result{}, err
			}
		}
	}

	var hostedClusterKubeconfigSecret corev1.Secret
	if err := r.Get(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      fmt.Sprintf("%s-admin-kubeconfig", req.NamespacedName.Name),
	}, &hostedClusterKubeconfigSecret); err != nil {
		log.Error(err, "unable to get kubeconfig secret for hostedcluster", "hostedcluster", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// create managedcluster namespace for hostedcluster
	if _, createErr := r.ControlplaneKubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.NamespacedName.Name,
		},
	}, metav1.CreateOptions{}); createErr != nil {
		if !errors.IsAlreadyExists(createErr) {
			log.Error(createErr, "unable to create managedcluster namespace for hostedcluster", "hostedcluster", req.NamespacedName)
			return ctrl.Result{}, createErr
		}
	}

	// create managedcluster-kubeconfig for hostedcluster
	if _, createErr := r.ControlplaneKubeClient.CoreV1().Secrets(req.NamespacedName.Name).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      helpers.ManagedClusterKubeConfig,
			Namespace: req.NamespacedName.Name,
		},
		Data: hostedClusterKubeconfigSecret.Data,
		Type: corev1.SecretTypeOpaque,
	}, metav1.CreateOptions{}); createErr != nil {
		if !errors.IsAlreadyExists(createErr) {
			log.Error(createErr, "unable to create managedcluster-kubeconfig for hostedcluster", "hostedcluster", req.NamespacedName)
			return ctrl.Result{}, createErr
		}
	}

	// create klusterlet for hostedcluster
	if _, createErr := r.ControlplaneOperatorClient.OperatorV1().Klusterlets().Create(ctx, &operatorapiv1.Klusterlet{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.NamespacedName.Name,
			Annotations: map[string]string{
				helpers.KlusterletOwnerAnnotation: "hostedcluster-controller",
			},
		},
		Spec: operatorapiv1.KlusterletSpec{
			DeployOption: operatorapiv1.KlusterletDeployOption{
				Mode: operatorapiv1.InstallModeHosted,
			},
		},
	}, metav1.CreateOptions{}); createErr != nil {
		if !errors.IsAlreadyExists(createErr) {
			log.Error(createErr, "unable to create klusterlet for hostedcluster", "hostedcluster", req.NamespacedName)
			return ctrl.Result{}, createErr
		}
	}

	return ctrl.Result{}, nil
}

func (r *HostedClusterReconciler) pruneManagedClusterResources(ctx context.Context, log logr.Logger, hostedCluster *hyperv1beta1.HostedCluster) error {
	// remove klusterlet for hosted cluster
	if err := r.ControlplaneOperatorClient.OperatorV1().Klusterlets().Delete(ctx, hostedCluster.GetName(), metav1.DeleteOptions{}); err != nil {
		if !errors.IsNotFound(err) {
			log.Error(err, "unable to delete klusterlet for hostedcluster", "hostedcluster", fmt.Sprintf("%s/%s", hostedCluster.GetNamespace(), hostedCluster.GetName()))
			return err
		}
	}

	return nil
}

func (r *HostedClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	hostedClusterPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if e.Object.GetAnnotations()[hostedClusterImportAnnotation] == "false" {
				return false
			}
			hcObject := e.Object.(*hyperv1beta1.HostedCluster)
			return meta.IsStatusConditionTrue(hcObject.Status.Conditions, string(hyperv1beta1.HostedClusterAvailable))
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			if e.ObjectNew.GetAnnotations()[hostedClusterImportAnnotation] == "false" {
				return false
			}
			if e.ObjectNew.GetResourceVersion() == e.ObjectOld.GetResourceVersion() {
				return false
			}
			hcObject := e.ObjectNew.(*hyperv1beta1.HostedCluster)
			return meta.IsStatusConditionTrue(hcObject.Status.Conditions, string(hyperv1beta1.HostedClusterAvailable))
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if e.Object.GetAnnotations()[hostedClusterImportAnnotation] == "false" {
				return false
			}
			return !e.DeleteStateUnknown
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&hyperv1beta1.HostedCluster{}, builder.WithPredicates(hostedClusterPredicate)).
		WithOptions(controller.Options{MaxConcurrentReconciles: 1}).
		Complete(r)
}

func containStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func removeStr(ss []string, s string) []string {
	var res []string
	for _, v := range ss {
		if v == s {
			continue
		}
		res = append(res, v)
	}

	return res
}
