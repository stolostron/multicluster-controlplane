package helpers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
)

func IsClusterUnavailable(ctx context.Context, clusterClient clusterclient.Interface, name string) (bool, error) {
	cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	if meta.IsStatusConditionFalse(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
		return true, nil
	}

	if meta.IsStatusConditionPresentAndEqual(
		cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown) {
		return true, nil
	}

	return false, nil
}

func DeleteManagedCluster(ctx context.Context, clusterClient clusterclient.Interface, clusterName string) error {
	managedCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, clusterName, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !managedCluster.DeletionTimestamp.IsZero() {
		return nil
	}

	if err := clusterClient.ClusterV1().ManagedClusters().Delete(ctx, clusterName, metav1.DeleteOptions{}); err != nil {
		return err
	}

	klog.Infof("The managed cluster %s is deleted", clusterName)
	return nil
}

func DeleteAllManifestWorks(
	ctx context.Context, workClient workclient.Interface, manifestWorks []workv1.ManifestWork, force bool) error {
	errs := []error{}
	for _, item := range manifestWorks {
		if force {
			if err := forceDeleteManifestWork(ctx, workClient, item.Namespace, item.Name); err != nil {
				errs = append(errs, err)
			}

			continue
		}

		if err := deleteManifestWork(ctx, workClient, item.Namespace, item.Name); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

func forceDeleteManifestWork(ctx context.Context, workClient workclient.Interface, namespace, name string) error {
	_, err := workClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if err := workClient.WorkV1().ManifestWorks(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	// reload the manifest work
	manifestWork, err := workClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	// if the manifest work is not deleted, force remove its finalizers
	if len(manifestWork.Finalizers) != 0 {
		patch := "{\"metadata\": {\"finalizers\":[]}}"
		if _, err := workClient.WorkV1().ManifestWorks(namespace).Patch(
			ctx, name, types.MergePatchType, []byte(patch), metav1.PatchOptions{}); err != nil {
			return err
		}
	}

	klog.Infof("The manifest work %s/%s is force deleted", namespace, name)
	return nil
}

func deleteManifestWork(ctx context.Context, workClient workclient.Interface, namespace, name string) error {
	manifestWork, err := workClient.WorkV1().ManifestWorks(namespace).Get(ctx, name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}

	if !manifestWork.DeletionTimestamp.IsZero() {
		// the manifest work is deleting, do nothing
		return nil
	}

	if err := workClient.WorkV1().ManifestWorks(namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		return err
	}

	klog.Infof("The manifest work %s/%s is deleted", namespace, name)
	return nil
}
