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

var (
	managementSharedStaticResourceFiles = []string{
		"klusterlet/management/klusterlet-agent-role.yaml",
		"klusterlet/management/klusterlet-agent-clusterrole.yaml",
	}

	managementStaticResourceFiles = []string{
		"klusterlet/management/klusterlet-agent-serviceaccount.yaml",
		"klusterlet/management/klusterlet-agent-rolebinding.yaml",
		"klusterlet/management/klusterlet-agent-clusterrolebinding.yaml",
	}
)

type managementReconcile struct {
	kubeClient kubernetes.Interface
	recorder   events.Recorder
	cache      resourceapply.ResourceCache
}

func (r *managementReconcile) reconcile(ctx context.Context, klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	err := ensureNamespace(ctx, r.kubeClient, klusterlet, config.AgentNamespace)
	if err != nil {
		return klusterlet, reconcileStop, err
	}

	resouceFiles := []string{}
	resouceFiles = append(resouceFiles, managementSharedStaticResourceFiles...)
	resouceFiles = append(resouceFiles, managementStaticResourceFiles...)

	resourceResults := helpers.ApplyDirectly(
		ctx,
		r.kubeClient,
		nil,
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
		resouceFiles...,
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
			Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "ManagementClusterResourceApplyFailed",
			Message: applyErrors.Error(),
		})
		return klusterlet, reconcileStop, applyErrors
	}

	return klusterlet, reconcileContinue, nil
}

func (r *managementReconcile) clean(ctx context.Context, klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	// Remove secrets
	secrets := []string{config.HubKubeConfigSecret}
	for _, secret := range secrets {
		err := r.kubeClient.CoreV1().Secrets(config.AgentNamespace).Delete(ctx, secret, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return klusterlet, reconcileStop, err
		}
		r.recorder.Eventf("SecretDeleted", "secret %s/%s is deleted", config.AgentNamespace, secret)
	}

	resouceFiles := []string{}
	if config.InstallMode != operatorapiv1.InstallModeHosted {
		resouceFiles = append(resouceFiles, managementSharedStaticResourceFiles...)
	}
	resouceFiles = append(resouceFiles, managementStaticResourceFiles...)

	// remove static file on the management cluster
	err := removeStaticResources(ctx, r.kubeClient, nil, resouceFiles, config)
	if err != nil {
		return klusterlet, reconcileStop, err
	}

	return klusterlet, reconcileContinue, nil
}
