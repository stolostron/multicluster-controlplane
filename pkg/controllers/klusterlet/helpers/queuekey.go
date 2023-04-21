// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"strings"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift/library-go/pkg/controller/factory"

	operatorlister "open-cluster-management.io/api/client/operator/listers/operator/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"
)

const (
	// KlusterletDefaultNamespace is the default namespace of klusterlet
	KlusterletDefaultNamespace = "open-cluster-management-agent"

	// BootstrapHubKubeConfig is the secret name of bootstrap kubeconfig secret to connect to hub
	BootstrapHubKubeConfig = "bootstrap-hub-kubeconfig"
	// HubKubeConfig is the secret name of kubeconfig secret to connect to hub with mtls
	HubKubeConfig = "hub-kubeconfig-secret"
	// ExternalManagedKubeConfig is the secret name of kubeconfig secret to connecting to the managed cluster
	// Only applicable to Hosted mode, klusterlet-operator uses it to install resources on the managed cluster.
	ExternalManagedKubeConfig      = "managedcluster-kubeconfig"
	ExternalManagedAgentKubeConfig = "external-managed-agent-kubeconfig"

	KlusterletReadyToApply = "ReadyToApply"
)

func KlusterletSecretQueueKeyFunc(klusterletLister operatorlister.KlusterletLister) factory.ObjectQueueKeyFunc {
	return func(obj runtime.Object) string {
		accessor, _ := meta.Accessor(obj)
		namespace := accessor.GetNamespace()
		name := accessor.GetName()
		interestedObjectFound := false
		if name == HubKubeConfig || name == BootstrapHubKubeConfig {
			interestedObjectFound = true
		}
		if !interestedObjectFound {
			return ""
		}

		klusterlets, err := klusterletLister.List(labels.Everything())
		if err != nil {
			return ""
		}

		if klusterlet := FindKlusterletByNamespace(klusterlets, namespace); klusterlet != nil {
			return klusterlet.Name
		}

		return ""
	}
}

func KlusterletDeploymentQueueKeyFunc(klusterletLister operatorlister.KlusterletLister) factory.ObjectQueueKeyFunc {
	return func(obj runtime.Object) string {
		accessor, _ := meta.Accessor(obj)
		namespace := accessor.GetNamespace()
		name := accessor.GetName()
		interestedObjectFound := false
		if strings.HasSuffix(name, "registration-agent") || strings.HasSuffix(name, "work-agent") {
			interestedObjectFound = true
		}
		if !interestedObjectFound {
			return ""
		}

		klusterlets, err := klusterletLister.List(labels.Everything())
		if err != nil {
			return ""
		}

		if klusterlet := FindKlusterletByNamespace(klusterlets, namespace); klusterlet != nil {
			return klusterlet.Name
		}

		return ""
	}
}

func FindKlusterletByNamespace(klusterlets []*operatorapiv1.Klusterlet, namespace string) *operatorapiv1.Klusterlet {
	for _, klusterlet := range klusterlets {
		agentNamespace := AgentNamespace(klusterlet)
		if namespace == agentNamespace {
			return klusterlet
		}
	}
	return nil
}
