// Copyright Contributors to the Open Cluster Management project
package klusterletcontroller

import (
	"context"
	"fmt"

	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"

	corev1 "k8s.io/api/core/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	appsinformer "k8s.io/client-go/informers/apps/v1"
	coreinformer "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	operatorinformer "open-cluster-management.io/api/client/operator/informers/externalversions/operator/v1"
	operatorlister "open-cluster-management.io/api/client/operator/listers/operator/v1"
	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
)

const (
	// klusterletHostedFinalizer is used to clean up resources on the managed/hosted cluster in Hosted mode
	klusterletHostedFinalizer    = "operator.open-cluster-management.io/klusterlet-hosted-cleanup"
	klusterletFinalizer          = "operator.open-cluster-management.io/klusterlet-cleanup"
	appliedManifestWorkFinalizer = "cluster.open-cluster-management.io/applied-manifest-work-cleanup"

	managedResourcesEvictionTimestampAnno = "operator.open-cluster-management.io/managed-resources-eviction-timestamp"

	klusterletReadyToApply = "ReadyToApply"
	klusterletApplied      = "Applied"
)

type klusterletController struct {
	kubeClient                kubernetes.Interface
	controlplaneKubeClient    kubernetes.Interface
	apiExtensionClient        apiextensionsclient.Interface
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface
	klusterletClient          operatorv1client.KlusterletInterface
	klusterletLister          operatorlister.KlusterletLister
	cache                     resourceapply.ResourceCache
}

type klusterletReconcile interface {
	reconcile(ctx context.Context, cm *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error)
	clean(ctx context.Context, cm *operatorapiv1.Klusterlet, config klusterletConfig) (*operatorapiv1.Klusterlet, reconcileState, error)
}

type reconcileState int64

const (
	reconcileStop reconcileState = iota
	reconcileContinue
)

// NewKlusterletController construct klusterlet controller
func NewKlusterletController(
	kubeClient kubernetes.Interface,
	controlplaneKubeClient kubernetes.Interface,
	apiExtensionClient apiextensionsclient.Interface,
	klusterletClient operatorv1client.KlusterletInterface,
	klusterletInformer operatorinformer.KlusterletInformer,
	secretInformer coreinformer.SecretInformer,
	deploymentInformer appsinformer.DeploymentInformer,
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface,
	recorder events.Recorder) factory.Controller {
	controller := &klusterletController{
		kubeClient:                kubeClient,
		controlplaneKubeClient:    controlplaneKubeClient,
		apiExtensionClient:        apiExtensionClient,
		appliedManifestWorkClient: appliedManifestWorkClient,
		klusterletClient:          klusterletClient,
		klusterletLister:          klusterletInformer.Lister(),
		cache:                     resourceapply.NewResourceCache(),
	}

	return factory.New().WithSync(controller.sync).
		WithInformersQueueKeyFunc(helpers.KlusterletDeploymentQueueKeyFunc(controller.klusterletLister), deploymentInformer.Informer()).
		WithInformersQueueKeyFunc(func(obj runtime.Object) string {
			accessor, _ := meta.Accessor(obj)
			return accessor.GetName()
		}, klusterletInformer.Informer()).
		ToController("KlusterletController", recorder)
}

// klusterletConfig is used to render the template of hub manifests
type klusterletConfig struct {
	KlusterletName string
	ClusterName    string

	// KlusterletNamespace is the namespace created on the managed cluster for each
	// klusterlet.
	// 1). In the Default mode, it refers to the same namespace as AgentNamespace;
	// 2). In the Hosted mode, the namespace still exists and contains some necessary
	//     resources for agents, like service accounts, roles and rolebindings.
	KlusterletNamespace string
	// AgentNamespace is the namespace to deploy the agents.
	// 1). In the Default mode, it is on the managed cluster and refers to the same
	//     namespace as KlusterletNamespace;
	// 2). In the Hosted mode, it is same with the controlplane namespace on the management cluster.
	AgentNamespace string

	HubKubeConfigSecret       string
	BootStrapKubeConfigSecret string

	AgentImage string

	ExternalManagedClusterKubeConfigSecret string

	InstallMode operatorapiv1.InstallMode

	// TODO support to configure standalone agent features
}

func (n *klusterletController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	klusterletName := controllerContext.QueueKey()
	klog.V(4).Infof("Reconciling Klusterlet %q", klusterletName)

	klusterlet, err := n.klusterletLister.Get(klusterletName)
	if errors.IsNotFound(err) {
		// Klusterlet not found, could have been deleted, do nothing.
		return nil
	}
	if err != nil {
		return err
	}

	klusterlet = klusterlet.DeepCopy()

	image, err := getAgentImage(ctx, n.kubeClient, klusterlet)
	if err != nil {
		return err
	}

	config := klusterletConfig{
		KlusterletName:                         klusterlet.Name,
		KlusterletNamespace:                    helpers.KlusterletNamespace(klusterlet),
		AgentNamespace:                         helpers.AgentNamespace(klusterlet),
		ClusterName:                            helpers.ClusterName(klusterlet),
		AgentImage:                             image,
		BootStrapKubeConfigSecret:              helpers.BootstrapHubKubeConfigSecret(klusterlet),
		HubKubeConfigSecret:                    helpers.HubKubeConfigSecret(klusterlet),
		ExternalManagedClusterKubeConfigSecret: helpers.ExternalManagedClusterKubeConfigSecret(klusterlet),
		InstallMode:                            klusterlet.Spec.DeployOption.Mode,
	}

	managedClusterClients, err := newManagedClusterClientsBuilder(
		klusterlet,
		n.kubeClient,
		n.controlplaneKubeClient,
		n.apiExtensionClient,
		n.appliedManifestWorkClient,
	).build(ctx)

	// update klusterletReadyToApply condition at first in hosted mode
	// this conditions should be updated even when klusterlet is in deleteing state.
	if config.InstallMode == operatorapiv1.InstallModeHosted {
		cond := metav1.Condition{
			Type: klusterletReadyToApply, Status: metav1.ConditionTrue, Reason: "KlusterletPrepared",
			Message: "Klusterlet is ready to apply",
		}
		if err != nil {
			cond = metav1.Condition{
				Type: klusterletReadyToApply, Status: metav1.ConditionFalse, Reason: "KlusterletPrepareFailed",
				Message: fmt.Sprintf("Failed to build managed cluster clients: %v", err),
			}
		}

		_, updated, updateErr := helpers.UpdateKlusterletStatus(ctx, n.klusterletClient, klusterletName,
			helpers.UpdateKlusterletConditionFn(cond))
		if updated {
			return updateErr
		}
	}

	if err != nil {
		return err
	}

	if !klusterlet.DeletionTimestamp.IsZero() {
		// The work of klusterlet cleanup will be handled by klusterlet cleanup controller
		return nil
	}

	// do nothing until finalizer is added.
	if !hasFinalizer(klusterlet, klusterletFinalizer) {
		return nil
	}

	// TODO support to configure standalone agent features

	reconcilers := []klusterletReconcile{
		&crdReconcile{
			managedClusterClients: managedClusterClients,
			recorder:              controllerContext.Recorder(),
			cache:                 n.cache},
		&managedReconcile{
			managedClusterClients: managedClusterClients,
			kubeClient:            n.kubeClient,
			recorder:              controllerContext.Recorder(),
			cache:                 n.cache},
		&managementReconcile{
			kubeClient: n.kubeClient,
			recorder:   controllerContext.Recorder(),
			cache:      n.cache},
		&runtimeReconcile{
			managedClusterClients: managedClusterClients,
			kubeClient:            n.kubeClient,
			recorder:              controllerContext.Recorder(),
			cache:                 n.cache},
	}

	var errs []error
	for _, reconciler := range reconcilers {
		var state reconcileState
		klusterlet, state, err = reconciler.reconcile(ctx, klusterlet, config)
		if err != nil {
			errs = append(errs, err)
		}
		if state == reconcileStop {
			break
		}
	}

	appliedCondition := meta.FindStatusCondition(klusterlet.Status.Conditions, klusterletApplied)
	if len(errs) == 0 {
		appliedCondition = &metav1.Condition{
			Type: klusterletApplied, Status: metav1.ConditionTrue, Reason: "KlusterletApplied",
			Message: "Klusterlet Component Applied"}
	} else {
		if appliedCondition == nil {
			appliedCondition = &metav1.Condition{
				Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "KlusterletApplyFailed",
				Message: "Klusterlet Component Apply failed"}
		}

		// When appliedCondition is false, we should not update related resources and resource generations
		_, updated, err := helpers.UpdateKlusterletStatus(ctx, n.klusterletClient, klusterletName,
			helpers.UpdateKlusterletConditionFn(*appliedCondition),
			func(oldStatus *operatorapiv1.KlusterletStatus) error {
				oldStatus.ObservedGeneration = klusterlet.Generation
				return nil
			},
		)

		if updated {
			return err
		}

		return utilerrors.NewAggregate(errs)
	}

	// If we get here, we have successfully applied everything.
	_, _, err = helpers.UpdateKlusterletStatus(ctx, n.klusterletClient, klusterletName,
		helpers.UpdateKlusterletConditionFn(*appliedCondition),
		helpers.UpdateKlusterletGenerationsFn(klusterlet.Status.Generations...),
		helpers.UpdateKlusterletRelatedResourcesFn(klusterlet.Status.RelatedResources...),
		func(oldStatus *operatorapiv1.KlusterletStatus) error {
			oldStatus.ObservedGeneration = klusterlet.Generation
			return nil
		},
	)
	return err
}

func ensureNamespace(ctx context.Context, kubeClient kubernetes.Interface, klusterlet *operatorapiv1.Klusterlet, namespace string) error {
	_, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	switch {
	case errors.IsNotFound(err):
		_, createErr := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
				Annotations: map[string]string{
					"workload.openshift.io/allowed": "management",
				},
			},
		}, metav1.CreateOptions{})
		meta.SetStatusCondition(&klusterlet.Status.Conditions, metav1.Condition{
			Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "KlusterletApplyFailed",
			Message: fmt.Sprintf("Failed to create namespace %q: %v", namespace, createErr)})
		return createErr

	case err != nil:
		meta.SetStatusCondition(&klusterlet.Status.Conditions, metav1.Condition{
			Type: klusterletApplied, Status: metav1.ConditionFalse, Reason: "KlusterletApplyFailed",
			Message: fmt.Sprintf("Failed to get namespace %q: %v", namespace, err)})
		return err
	}

	return nil
}

func getAgentImage(ctx context.Context, kubeClient kubernetes.Interface, klusterlet *operatorapiv1.Klusterlet) (string, error) {
	if klusterlet.Spec.DeployOption.Mode == operatorapiv1.InstallModeHosted {
		namespace := helpers.GetComponentNamespace()
		name := "multicluster-controlplane"
		deploy, err := kubeClient.AppsV1().Deployments(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return "", err
		}

		for _, c := range deploy.Spec.Template.Spec.Containers {
			if c.Name == "controlplane" {
				return c.Image, nil
			}
		}

		return "", fmt.Errorf("faild to find current controlplane image from `controlplane` container in deployment %s", name)
	}

	// if klusterlet is in default mode, the management agent need not to be deployed
	return "", nil
}
