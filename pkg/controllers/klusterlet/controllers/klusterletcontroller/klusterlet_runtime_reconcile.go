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
	"k8s.io/client-go/kubernetes"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/manifests"
)

// runtimeReconcile ensure all runtime of klusterlet is applied
type runtimeReconcile struct {
	managedClusterClients *managedClusterClients
	kubeClient            kubernetes.Interface
	recorder              events.Recorder
	cache                 resourceapply.ResourceCache
}

func (r *runtimeReconcile) reconcile(ctx context.Context,
	klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	if config.InstallMode == operatorapiv1.InstallModeHosted {
		// Create managed config secret for agent.
		if err := r.createManagedClusterKubeconfig(
			ctx,
			klusterlet,
			config.KlusterletNamespace,
			config.AgentNamespace,
			fmt.Sprintf("%s-agent-sa", klusterlet.Name),
			config.ExternalManagedClusterKubeConfigSecret,
			r.recorder,
		); err != nil {
			return klusterlet, reconcileStop, err
		}
	}

	// Deploy registration agent
	_, generationStatus, err := helpers.ApplyDeployment(
		ctx,
		r.kubeClient,
		klusterlet.Status.Generations,
		klusterlet.Spec.NodePlacement,
		func(name string) ([]byte, error) {
			template, err := manifests.KlusterletManifestFiles.ReadFile(name)
			if err != nil {
				return nil, err
			}
			objData := assets.MustCreateAssetFromTemplate(name, template, config).Data
			helpers.SetRelatedResourcesStatusesWithObj(&klusterlet.Status.RelatedResources, objData)
			return objData, nil
		},
		r.recorder,
		"klusterlet/management/klusterlet-agent-deployment.yaml",
	)
	if err != nil {
		return klusterlet, reconcileStop, err
	}

	helpers.SetGenerationStatuses(&klusterlet.Status.Generations, generationStatus)
	return klusterlet, reconcileContinue, nil
}

func (r *runtimeReconcile) createManagedClusterKubeconfig(
	ctx context.Context,
	klusterlet *operatorapiv1.Klusterlet,
	klusterletNamespace, agentNamespace, saName, secretName string,
	recorder events.Recorder) error {
	tokenGetter := helpers.SATokenGetter(ctx, saName, klusterletNamespace, r.managedClusterClients.kubeClient)
	err := helpers.SyncKubeConfigSecret(
		ctx,
		secretName,
		agentNamespace,
		"/spoke/config/kubeconfig",
		r.managedClusterClients.kubeconfig,
		r.kubeClient.CoreV1(),
		tokenGetter,
		recorder,
	)
	if err != nil {
		meta.SetStatusCondition(&klusterlet.Status.Conditions, metav1.Condition{
			Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "KlusterletApplyFailed",
			Message: fmt.Sprintf("Failed to create managed kubeconfig secret %s with error %v", secretName, err),
		})
	}
	return err
}

func (r *runtimeReconcile) clean(ctx context.Context,
	klusterlet *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error) {
	deployments := []string{fmt.Sprintf("%s-multicluster-controlplane-agent", config.KlusterletName)}
	for _, deployment := range deployments {
		err := r.kubeClient.AppsV1().Deployments(config.AgentNamespace).Delete(ctx, deployment, metav1.DeleteOptions{})
		if err != nil && !errors.IsNotFound(err) {
			return klusterlet, reconcileStop, err
		}
		r.recorder.Eventf("DeploymentDeleted", "deployment %s is deleted", deployment)
	}

	return klusterlet, reconcileContinue, nil
}
