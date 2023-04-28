// Copyright Contributors to the Open Cluster Management project
package klusterletcontroller

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/assets"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/kubernetes"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/manifests"
)

var managedStaticResourceFiles = []string{
	"klusterlet/managed/klusterlet-agent-serviceaccount.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrole.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrole-addon-management.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrole-execution.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrolebinding.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrolebinding-addon-management.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrolebinding-execution.yaml",
	"klusterlet/managed/klusterlet-agent-clusterrolebinding-execution-admin.yaml",
}

// managedReconcile apply resources to managed clusters
type managedReconcile struct {
	managedClusterClients *managedClusterClients
	kubeClient            kubernetes.Interface
	recorder              events.Recorder
	cache                 resourceapply.ResourceCache
}

func (r *managedReconcile) reconcile(ctx context.Context, klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	if config.InstallMode == operatorapiv1.InstallModeHosted {
		// In hosted mode, ensure the klusterlet namespaces (open-cluster-management-<hosted-cluster>) on the managed
		// cluster for setting the rabc of agent
		if err := ensureNamespace(ctx, r.managedClusterClients.kubeClient, klusterlet, config.KlusterletNamespace); err != nil {
			return klusterlet, reconcileStop, err
		}
	}

	managedResource := managedStaticResourceFiles
	resourceResults := helpers.ApplyDirectly(
		ctx,
		r.managedClusterClients.kubeClient,
		r.managedClusterClients.apiExtensionClient,
		r.recorder,
		r.cache,
		func(name string) ([]byte, error) {
			template, err := manifests.KlusterletManifestFiles.ReadFile(name)
			if err != nil {
				return nil, err
			}
			objData := assets.MustCreateAssetFromTemplate(name, template, config).Data
			helpers.SetRelatedResourcesStatusesWithObj(&klusterlet.Status.RelatedResources, objData)
			return objData, nil
		},
		managedResource...,
	)

	var errs []error
	for _, result := range resourceResults {
		if result.Error != nil {
			errs = append(errs, fmt.Errorf("%q (%T): %v", result.File, result.Type, result.Error))
		}
	}

	if len(errs) > 0 {
		applyErrors := utilerrors.NewAggregate(errs)
		meta.SetStatusCondition(&klusterlet.Status.Conditions, metav1.Condition{
			Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "ManagedClusterResourceApplyFailed",
			Message: applyErrors.Error(),
		})
		return klusterlet, reconcileStop, applyErrors
	}

	return klusterlet, reconcileContinue, nil
}

func (r *managedReconcile) clean(ctx context.Context, klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	// nothing should be done when deploy mode is hosted and hosted finalizer is not added.
	if klusterlet.Spec.DeployOption.Mode == operatorapiv1.InstallModeHosted && !hasFinalizer(klusterlet, klusterletHostedFinalizer) {
		return klusterlet, reconcileContinue, nil
	}

	if err := r.cleanUpAppliedManifestWorks(ctx, klusterlet, config); err != nil {
		return klusterlet, reconcileStop, err
	}

	if err := removeStaticResources(ctx, r.managedClusterClients.kubeClient, r.managedClusterClients.apiExtensionClient,
		managedStaticResourceFiles, config); err != nil {
		return klusterlet, reconcileStop, err
	}

	// remove the klusterlet namespace and klusterlet addon namespace on the managed cluster
	// For now, whether in Default or Hosted mode, the addons could be deployed on the managed cluster.
	namespaces := []string{config.KlusterletNamespace, fmt.Sprintf("%s-addon", config.KlusterletNamespace)}
	for _, namespace := range namespaces {
		if err := r.managedClusterClients.kubeClient.CoreV1().Namespaces().Delete(
			ctx, namespace, metav1.DeleteOptions{}); err != nil && !errors.IsNotFound(err) {
			return klusterlet, reconcileStop, err
		}
	}

	return klusterlet, reconcileContinue, nil
}

// cleanUpAppliedManifestWorks removes finalizer from the AppliedManifestWorks whose name starts with
// the hash of the given hub host.
func (r *managedReconcile) cleanUpAppliedManifestWorks(ctx context.Context, klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) error {
	appliedManifestWorks, err := r.managedClusterClients.appliedManifestWorkClient.List(ctx, metav1.ListOptions{})
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to list AppliedManifestWorks: %w", err)
	}

	if len(appliedManifestWorks.Items) == 0 {
		return nil
	}

	var errs []error
	for index := range appliedManifestWorks.Items {
		// ignore AppliedManifestWork for other klusterlet
		if string(klusterlet.UID) != appliedManifestWorks.Items[index].Spec.AgentID {
			continue
		}

		// remove finalizer if exists
		if mutated := removeFinalizer(&appliedManifestWorks.Items[index], appliedManifestWorkFinalizer); !mutated {
			continue
		}

		_, err := r.managedClusterClients.appliedManifestWorkClient.Update(ctx, &appliedManifestWorks.Items[index], metav1.UpdateOptions{})
		if err != nil && !errors.IsNotFound(err) {
			errs = append(errs, fmt.Errorf("unable to remove finalizer from AppliedManifestWork %q: %w", appliedManifestWorks.Items[index].Name, err))
		}
	}
	return utilerrors.NewAggregate(errs)
}
