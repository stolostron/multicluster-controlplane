package clusterinfo

import (
	"context"
	"sort"
	"time"

	openshiftclientset "github.com/openshift/client-go/config/clientset/versioned"
	clusterinfoclient "github.com/stolostron/cluster-lifecycle-api/client/clusterinfo/clientset/versioned"
	clusterv1beta1infolister "github.com/stolostron/cluster-lifecycle-api/client/clusterinfo/listers/clusterinfo/v1beta1"
	clusterv1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	clusterv1alpha1lister "open-cluster-management.io/api/client/cluster/listers/cluster/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-controlplane/pkg/helpers"
)

// ClusterInfoReconciler reconciles a ManagedClusterInfo object
type ClusterInfoReconciler struct {
	client.Client
	ManagedClusterInfoClient clusterinfoclient.Interface
	ManagedClusterClient     kubernetes.Interface
	ClaimLister              clusterv1alpha1lister.ClusterClaimLister
	ManagedClusterInfoList   clusterv1beta1infolister.ManagedClusterInfoLister
	ConfigV1Client           openshiftclientset.Interface
	ClusterName              string
}

type clusterInfoStatusSyncer interface {
	sync(ctx context.Context, clusterInfo *clusterv1beta1.ManagedClusterInfo) error
}

func (r *ClusterInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	clusterInfo, err := r.ManagedClusterInfoList.ManagedClusterInfos(r.ClusterName).Get(r.ClusterName)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if helpers.ClusterIsOffLine(clusterInfo.Status.Conditions) {
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	newClusterInfo := clusterInfo.DeepCopy()

	syncers := []clusterInfoStatusSyncer{
		&defaultInfoSyncer{
			claimLister: r.ClaimLister,
		},
		&distributionInfoSyncer{
			configV1Client:       r.ConfigV1Client,
			managedClusterClient: r.ManagedClusterClient,
			claimLister:          r.ClaimLister,
		},
	}

	var errs []error
	for _, s := range syncers {
		if err := s.sync(ctx, newClusterInfo); err != nil {
			errs = append(errs, err)
		}
	}

	newSyncedCondition := metav1.Condition{
		Type:    clusterv1beta1.ManagedClusterInfoSynced,
		Status:  metav1.ConditionTrue,
		Reason:  clusterv1beta1.ReasonManagedClusterInfoSynced,
		Message: "Managed cluster info is synced",
	}
	if len(errs) > 0 {
		newSyncedCondition = metav1.Condition{
			Type:    clusterv1beta1.ManagedClusterInfoSynced,
			Status:  metav1.ConditionFalse,
			Reason:  clusterv1beta1.ReasonManagedClusterInfoSyncedFailed,
			Message: errors.NewAggregate(errs).Error(),
		}
	}
	meta.SetStatusCondition(&newClusterInfo.Status.Conditions, newSyncedCondition)

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		clusterInfo, err := r.ManagedClusterInfoClient.InternalV1beta1().ManagedClusterInfos(r.ClusterName).Get(ctx, r.ClusterName, metav1.GetOptions{})
		if err != nil {
			return client.IgnoreNotFound(err)
		}

		if !clusterInfoStatusUpdated(&clusterInfo.Status, &newClusterInfo.Status) {
			return nil
		}

		clusterInfo.Status = newClusterInfo.Status

		_, err = r.ManagedClusterInfoClient.InternalV1beta1().ManagedClusterInfos(r.ClusterName).UpdateStatus(ctx, clusterInfo, metav1.UpdateOptions{})
		return err
	}); err != nil {
		klog.Errorf("Failed to update clusterInfo status. error %v", err)
		return ctrl.Result{}, err
	}

	// need to sync ocp ClusterVersion info every 5 min since do not watch it.
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

func clusterInfoStatusUpdated(old, new *clusterv1beta1.ClusterInfoStatus) bool {

	switch new.DistributionInfo.Type {
	case clusterv1beta1.DistributionTypeOCP:
		// sort the slices in distributionInfo to make it comparable using DeepEqual if not update.
		if ocpDistributionInfoUpdated(&old.DistributionInfo.OCP, &new.DistributionInfo.OCP) {
			return true
		}
	}
	return !equality.Semantic.DeepEqual(old, new)
}

func ocpDistributionInfoUpdated(old, new *clusterv1beta1.OCPDistributionInfo) bool {
	sort.SliceStable(new.AvailableUpdates, func(i, j int) bool { return new.AvailableUpdates[i] < new.AvailableUpdates[j] })
	sort.SliceStable(new.VersionAvailableUpdates, func(i, j int) bool {
		return new.VersionAvailableUpdates[i].Version < new.VersionAvailableUpdates[j].Version
	})
	sort.SliceStable(new.VersionHistory, func(i, j int) bool { return new.VersionHistory[i].Version < new.VersionHistory[j].Version })
	return !equality.Semantic.DeepEqual(old, new)
}
