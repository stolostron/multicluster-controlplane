package clusterimport

import (
	"bytes"
	"context"
	"fmt"

	khelpers "github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"

	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	operatorv1listers "open-cluster-management.io/api/client/operator/listers/operator/v1"
	operatorv1 "open-cluster-management.io/api/operator/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

/* #nosec */
const ManagedClusterKubeConfigSecretName string = "managedcluster-kubeconfig"

type ClusterImportController struct {
	kubeClient       kubernetes.Interface
	klusterletClient operatorv1client.KlusterletInterface
	klusterletLister operatorv1listers.KlusterletLister
	secretLister     corev1listers.SecretLister
}

// blank assignment to verify that ManagedClusterImportController implements reconcile.Reconciler
var _ reconcile.Reconciler = &ClusterImportController{}

func (c *ClusterImportController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	clusterName := request.Name

	klusterlet, err := c.klusterletLister.Get(clusterName)
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	// only hanld hosted cluster
	if klusterlet.Spec.DeployOption.Mode != operatorv1.InstallModeHosted {
		return reconcile.Result{}, nil
	}

	if !klusterlet.DeletionTimestamp.IsZero() {
		// TODO clean up the managed cluster
		return reconcile.Result{}, nil
	}

	managedClusterSecret, err := c.secretLister.Secrets(clusterName).Get(ManagedClusterKubeConfigSecretName)
	if errors.IsNotFound(err) {
		klog.Warningf("the managed cluster kubeconfig secret of %s is not found , %v", clusterName, err)
		_, updated, updateErr := khelpers.UpdateKlusterletStatus(
			ctx,
			c.klusterletClient,
			clusterName,
			khelpers.UpdateKlusterletConditionFn(metav1.Condition{
				Type:    khelpers.KlusterletReadyToApply,
				Status:  metav1.ConditionFalse,
				Reason:  "ManagedClusterKubeconfigSecretMissed",
				Message: "Managed cluster kubeconfig secret in not found in the managed cluster namespace",
			}),
		)
		if updated {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if err := c.validateManagedClusterSecret(managedClusterSecret); err != nil {
		klog.Warningf("the managed cluster kubeconfig secret of %s is invalid , %v", clusterName, err)
		_, updated, updateErr := khelpers.UpdateKlusterletStatus(
			ctx,
			c.klusterletClient,
			clusterName,
			khelpers.UpdateKlusterletConditionFn(metav1.Condition{
				Type:    khelpers.KlusterletReadyToApply,
				Status:  metav1.ConditionFalse,
				Reason:  "ManagedClusterKubeconfigSecretInvalid",
				Message: fmt.Sprintf("The kubeconfig is invalid, %v", err),
			}),
		)
		if updated {
			return reconcile.Result{}, updateErr
		}
		return reconcile.Result{}, nil
	}

	if err := c.syncSecret(ctx, clusterName, managedClusterSecret); err != nil {
		return reconcile.Result{}, err
	}

	// TODO we may also add hosted annotation to the managed cluster for supporting hosted addon

	return reconcile.Result{}, nil
}

func (c *ClusterImportController) validateManagedClusterSecret(secret *corev1.Secret) error {
	if len(secret.Data["kubeconfig"]) == 0 {
		return fmt.Errorf("the kubeconfig is not found")
	}

	// TODO check the managed cluster kubeconfig permissions
	return nil
}

func (c *ClusterImportController) syncSecret(ctx context.Context, clusterName string, secret *corev1.Secret) error {
	hostedSecretName := fmt.Sprintf("%s-external-managed-kubeconfig", clusterName)
	namespace := khelpers.GetComponentNamespace()
	hostedSecret, err := c.kubeClient.CoreV1().Secrets(namespace).Get(ctx, hostedSecretName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		_, err := c.kubeClient.CoreV1().Secrets(namespace).Create(
			ctx,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: hostedSecretName,
				},
				Data: map[string][]byte{
					"kubeconfig": secret.Data["kubeconfig"],
				},
			},
			metav1.CreateOptions{},
		)

		return err
	}
	if err != nil {
		return err
	}

	if bytes.Equal(hostedSecret.Data["kubeconfig"], secret.Data["kubeconfig"]) {
		return nil
	}

	hostedSecret.Data["kubeconfig"] = secret.Data["kubeconfig"]
	_, err = c.kubeClient.CoreV1().Secrets(namespace).Update(ctx, hostedSecret, metav1.UpdateOptions{})
	return err
}
