package addons

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"open-cluster-management.io/managed-serviceaccount/pkg/addon/agent/controller"
	"open-cluster-management.io/managed-serviceaccount/pkg/common"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/helpers"
)

func StartManagedServiceAccountAgent(ctx context.Context, hubMgr manager.Manager, clusterName string) error {
	spokeNamespace := helpers.GetComponentNamespace()

	hubNativeClient, err := kubernetes.NewForConfig(hubMgr.GetConfig())
	if err != nil {
		return fmt.Errorf("unable to instantiate a kubernetes native client")
	}

	spokeCfg, err := rest.InClusterConfig()
	if err != nil {
		return fmt.Errorf("failed build a in-cluster spoke cluster client config")
	}

	spokeNativeClient, err := kubernetes.NewForConfig(spokeCfg)
	if err != nil {
		return fmt.Errorf("unable to build a spoke kubernetes client")
	}

	resources, err := spokeNativeClient.Discovery().ServerResourcesForGroupVersion("v1")
	if err != nil {
		return fmt.Errorf("failed api discovery in the spoke cluster: %v", err)
	}
	found := false
	for _, r := range resources.APIResources {
		if r.Kind == "TokenRequest" {
			found = true
		}
	}
	if !found {
		return fmt.Errorf(`no "serviceaccounts/token" resource discovered in the managed cluster,` +
			`is --service-account-signing-key-file configured for the kube-apiserver?`)
	}

	spokeCache, err := cache.New(spokeCfg, cache.Options{
		SelectorsByObject: cache.SelectorsByObject{
			&corev1.ServiceAccount{}: {
				Label: labels.SelectorFromSet(
					labels.Set{
						common.LabelKeyIsManagedServiceAccount: "true",
					},
				),
			},
		},
		Namespace: spokeNamespace,
	})
	if err != nil {
		return fmt.Errorf("unable to instantiate a spoke serviceaccount cache")
	}
	if err = hubMgr.Add(spokeCache); err != nil {
		return fmt.Errorf("unable to add spoke cache to manager")
	}

	ctrl := controller.TokenReconciler{
		ClusterName:       clusterName,
		Cache:             hubMgr.GetCache(),
		HubClient:         hubMgr.GetClient(),
		HubNativeClient:   hubNativeClient,
		SpokeNamespace:    spokeNamespace,
		SpokeNativeClient: spokeNativeClient,
		SpokeClientConfig: spokeCfg,
		SpokeCache:        spokeCache,
	}

	return ctrl.SetupWithManager(hubMgr)
}
