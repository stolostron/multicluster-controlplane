// Copyright Contributors to the Open Cluster Management project
package controller

import (
	"context"
	_ "net/http/pprof"
	"time"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonclient "open-cluster-management.io/api/client/addon/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policyv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	appsv1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/clustermanagementaddons"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/managedclusteraddons"
)

var ResyncInterval = 5 * time.Minute

// InstallManagedClusterAddons to install managed-serviceaccount and policy addons in managed cluster
func InstallManagedClusterAddons(ctx context.Context, kubeConfig *rest.Config, kubeClient kubernetes.Interface) error {
	addonManager, err := addonmanager.New(kubeConfig)
	if err != nil {
		return err
	}
	addonClient, err := addonclient.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	if err := managedclusteraddons.AddPolicyAddons(addonManager, kubeConfig, kubeClient, addonClient); err != nil {
		return err
	}

	if err := managedclusteraddons.AddManagedServiceAccountAddon(addonManager, kubeClient, addonClient); err != nil {
		return err
	}

	if err := managedclusteraddons.AddManagedClusterInfoAddon(addonManager, kubeClient, addonClient); err != nil {
		return err
	}

	if err := addonManager.Start(ctx); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// InstallClusterManagmentAddons installs managed-serviceaccount and policy addons in hub cluster
func InstallClusterManagmentAddons(ctx context.Context, kubeConfig *rest.Config,
	kubeClient kubernetes.Interface, dynamicClient dynamic.Interface,
) error {
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(authv1alpha1.AddToScheme(scheme))
	// policy propagator required
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(policyv1beta1.AddToScheme(scheme))
	// managed cluster info
	utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))

	ctrl.SetLogger(klogr.New())

	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		klog.Error("unable to start manager", err)
		return err
	}

	klog.Info("finish new InstallClusterManagmentAddons")

	if err := clustermanagementaddons.SetupClusterInfoWithManager(mgr, kubeClient); err != nil {
		return err
	}

	if err := clustermanagementaddons.SetupManagedServiceAccountWithManager(mgr); err != nil {
		return err
	}

	if err := clustermanagementaddons.SetupPolicyWithManager(ctx, mgr, kubeConfig, kubeClient, dynamicClient); err != nil {
		return err
	}

	if err := mgr.Start(ctx); err != nil {
		return err
	}

	<-ctx.Done()
	return nil
}
