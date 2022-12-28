// Copyright Contributors to the Open Cluster Management project
package controller

import (
	"fmt"
	_ "net/http/pprof"
	"time"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonclient "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policyv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	placementrulecontroller "open-cluster-management.io/multicloud-operators-subscription/pkg/placementrule/controller"
	ocmcrds "open-cluster-management.io/multicluster-controlplane/config/crds"
	"open-cluster-management.io/multicluster-controlplane/pkg/controllers/ocmcontroller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-controlplane/config/crds"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/clustermanagementaddons"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/managedclusteraddons"
)

var ResyncInterval = 5 * time.Minute

func InstallAddonCrds(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	klog.Info("installing ocm addon crds")
	apiextensionsClient, err := apiextensionsclient.NewForConfig(aggregatorConfig.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return err
	}
	dynamicClient, err := dynamic.NewForConfig(aggregatorConfig.GenericConfig.LoopbackClientConfig)
	if err != nil {
		return err
	}
	ctx := ocmcontroller.GoContext(stopCh)
	if !ocmcrds.WaitForOcmCrdReady(ctx, dynamicClient) {
		return fmt.Errorf("ocm crds is not ready")
	}
	if err := crds.Bootstrap(ctx, apiextensionsClient); err != nil {
		klog.Errorf("failed to bootstrap ocm addon crds: %v", err)
		// nolint:nilerr
		return nil // don't klog.Fatal. This only happens when context is cancelled.
	}
	klog.Info("installed ocm addon crds")
	return nil
}

// InstallManagedClusterAddons to install managed-serviceaccount and policy addons in managed cluster
func InstallManagedClusterAddons(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	restConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
	restConfig.ContentType = "application/json"

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	go func() {
		ctx := ocmcontroller.GoContext(stopCh)

		if !crds.WaitForOcmAddonCrdsReady(ctx, dynamicClient) {
			klog.Errorf("ocm addon crds is not ready")
		}

		addonManager, err := addonmanager.New(restConfig)
		if err != nil {
			klog.Error(err)
		}
		addonClient, err := addonclient.NewForConfig(restConfig)
		if err != nil {
			klog.Error(err)
		}
		if err := managedclusteraddons.AddPolicyAddons(addonManager, restConfig, kubeClient, addonClient); err != nil {
			klog.Error(err)
		}

		if err := managedclusteraddons.AddManagedServiceAccountAddon(addonManager, kubeClient, addonClient); err != nil {
			klog.Error(err)
		}
		if err := managedclusteraddons.AddManagedClusterInfoAddon(addonManager, kubeClient, addonClient); err != nil {
			klog.Error(err)
		}

		if err := addonManager.Start(ctx); err != nil {
			klog.Errorf("failed to start managedcluster addons: %v", err)
		}
		<-ctx.Done()
	}()

	return nil
}

// InstallClusterManagementAddons installs managed-serviceaccount and policy addons in hub cluster
func InstallClusterManagementAddons(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	restConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
	restConfig.ContentType = "application/json"

	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return err
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	go func() {
		ctx := ocmcontroller.GoContext(stopCh)

		if !crds.WaitForOcmAddonCrdsReady(ctx, dynamicClient) {
			klog.Errorf("ocm addon crds is not ready")
			return
		}

		scheme := runtime.NewScheme()
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		utilruntime.Must(authv1alpha1.AddToScheme(scheme))
		// policy propagator required
		utilruntime.Must(clusterv1.AddToScheme(scheme))
		utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
		utilruntime.Must(policyv1.AddToScheme(scheme))
		utilruntime.Must(policyv1beta1.AddToScheme(scheme))
		// managed cluster info
		utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))
		// placementrule
		utilruntime.Must(placementrulev1.AddToScheme(scheme))

		ctrl.SetLogger(klogr.New())

		mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
			Scheme: scheme,
		})
		if err != nil {
			klog.Errorf("unable to start manager %v", err)
		}

		klog.Info("finish new InstallClusterManagementAddons")

		if err := clustermanagementaddons.SetupClusterInfoWithManager(mgr); err != nil {
			klog.Error(err)
		}

		if err := clustermanagementaddons.SetupManagedServiceAccountWithManager(mgr); err != nil {
			klog.Error(err)
		}

		if err := clustermanagementaddons.SetupPolicyWithManager(ctx, mgr, restConfig, kubeClient, dynamicClient); err != nil {
			klog.Error(err)
		}

		// placementrule controller
		if err := placementrulecontroller.AddToManager(mgr); err != nil {
			klog.Error(err)
		}

		if err := mgr.Start(ctx); err != nil {
			klog.Error(err)
		}

		<-ctx.Done()
	}()

	return nil
}
