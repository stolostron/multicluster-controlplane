package clusterimport

import (
	"context"

	clusterv1 "open-cluster-management.io/api/cluster/v1"

	certificatesv1 "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const clusterLabel = "open-cluster-management.io/cluster-name"

type CSRApprovalController struct {
	runtimeClient client.Client
	kubeClient    kubernetes.Interface
}

var _ reconcile.Reconciler = &CSRApprovalController{}

func (r *CSRApprovalController) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	csrReq := r.kubeClient.CertificatesV1().CertificateSigningRequests()
	csr, err := csrReq.Get(ctx, request.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}
	if isCSRInTerminalState(&csr.Status) {
		return reconcile.Result{}, nil
	}

	clusterName := getClusterName(csr)
	cluster := &clusterv1.ManagedCluster{}
	err = r.runtimeClient.Get(ctx, types.NamespacedName{Name: clusterName}, cluster)
	if errors.IsNotFound(err) {
		// no managed cluster, do nothing.
		return reconcile.Result{}, nil
	}
	if err != nil {
		return reconcile.Result{}, err
	}

	if !cluster.Spec.HubAcceptsClient {
		cluster.Spec.HubAcceptsClient = true
		err = r.runtimeClient.Update(ctx, cluster)
		if errors.IsNotFound(err) {
			// no managed cluster, do nothing.
			return reconcile.Result{}, nil
		}
		if err != nil {
			return reconcile.Result{}, err
		}
	}

	csr = csr.DeepCopy()
	csr.Status.Conditions = append(csr.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:           certificatesv1.CertificateApproved,
		Status:         corev1.ConditionTrue,
		Reason:         "AutoApprovedByImportController",
		Message:        "The managedcluster-import-controller approves this CSR automatically",
		LastUpdateTime: metav1.Now(),
	})
	if _, err := csrReq.UpdateApproval(ctx, csr.Name, csr, metav1.UpdateOptions{}); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// check whether a CSR is in terminal state
func isCSRInTerminalState(status *certificatesv1.CertificateSigningRequestStatus) bool {
	for _, c := range status.Conditions {
		if c.Type == certificatesv1.CertificateApproved {
			return true
		}
		if c.Type == certificatesv1.CertificateDenied {
			return true
		}
	}
	return false
}

func getClusterName(csr *certificatesv1.CertificateSigningRequest) (clusterName string) {
	for label, v := range csr.GetObjectMeta().GetLabels() {
		if label == clusterLabel {
			clusterName = v
		}
	}
	return clusterName
}
