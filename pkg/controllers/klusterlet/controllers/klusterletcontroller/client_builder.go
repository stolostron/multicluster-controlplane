// Copyright Contributors to the Open Cluster Management project
package klusterletcontroller

import (
	"context"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	workclientset "open-cluster-management.io/api/client/work/clientset/versioned"
	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
)

// managedClusterClients holds variety of kube client for managed cluster
type managedClusterClients struct {
	kubeClient                kubernetes.Interface
	apiExtensionClient        apiextensionsclient.Interface
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface
	// Only used for Hosted mode to generate managed cluster kubeconfig
	// with minimum permission for registration and work.
	kubeconfig *rest.Config
}

type managedClusterClientsBuilder struct {
	klusterlet *operatorapiv1.Klusterlet

	kubeClient                kubernetes.Interface
	controlplaneKubeClient    kubernetes.Interface
	apiExtensionClient        apiextensionsclient.Interface
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface
}

func newManagedClusterClientsBuilder(
	klusterlet *operatorapiv1.Klusterlet,
	kubeClient kubernetes.Interface,
	controlplaneKubeClient kubernetes.Interface,
	apiExtensionClient apiextensionsclient.Interface,
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface,
) *managedClusterClientsBuilder {
	return &managedClusterClientsBuilder{
		klusterlet:                klusterlet,
		kubeClient:                kubeClient,
		controlplaneKubeClient:    controlplaneKubeClient,
		apiExtensionClient:        apiExtensionClient,
		appliedManifestWorkClient: appliedManifestWorkClient,
	}
}

func (m *managedClusterClientsBuilder) build(ctx context.Context) (*managedClusterClients, error) {
	if m.klusterlet.Spec.DeployOption.Mode != operatorapiv1.InstallModeHosted {
		return &managedClusterClients{
			kubeClient:                m.kubeClient,
			apiExtensionClient:        m.apiExtensionClient,
			appliedManifestWorkClient: m.appliedManifestWorkClient,
		}, nil
	}

	managedKubeconfigSecret, err := m.controlplaneKubeClient.CoreV1().Secrets(helpers.ClusterName(m.klusterlet)).Get(
		ctx, helpers.ManagedClusterKubeConfig, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	managedKubeConfig, err := helpers.LoadClientConfigFromSecret(managedKubeconfigSecret)
	if err != nil {
		return nil, err
	}

	clients := &managedClusterClients{
		kubeconfig: managedKubeConfig,
	}

	if clients.kubeClient, err = kubernetes.NewForConfig(managedKubeConfig); err != nil {
		return nil, err
	}

	if clients.apiExtensionClient, err = apiextensionsclient.NewForConfig(managedKubeConfig); err != nil {
		return nil, err
	}

	workClient, err := workclientset.NewForConfig(managedKubeConfig)
	if err != nil {
		return nil, err
	}

	clients.appliedManifestWorkClient = workClient.WorkV1().AppliedManifestWorks()
	return clients, nil
}
