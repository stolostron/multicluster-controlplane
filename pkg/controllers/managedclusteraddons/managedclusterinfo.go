// Copyright Contributors to the Open Cluster Management project

package managedclusteraddons

import (
	"github.com/stolostron/multicloud-operators-foundation/pkg/addon"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/api/client/addon/clientset/versioned"
)

func AddManagedClusterInfoAddon(addonManager addonmanager.AddonManager, kubeClient kubernetes.Interface, addonClient versioned.Interface) error {
	registrationOption := addon.NewRegistrationOption(kubeClient, addon.WorkManagerAddonName)
	agentAddon, err := addonfactory.NewAgentAddonFactory(addon.WorkManagerAddonName, addon.ChartFS, addon.ChartDir).
		WithConfigGVRs(addonfactory.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			addon.NewGetValuesFunc("quay.io/stolostron/multicloud-manager:latest"),
			addonfactory.GetValuesFromAddonAnnotation,
			addonfactory.GetAddOnDeloymentConfigValues(
				addonfactory.NewAddOnDeloymentConfigGetter(addonClient),
				addonfactory.ToAddOnNodePlacementValues,
			),
		).
		WithAgentRegistrationOption(registrationOption).
		WithInstallStrategy(agent.InstallAllStrategy("open-cluster-management-agent-addon")).
		BuildHelmAgentAddon()
	if err != nil {
		klog.Errorf("failed to build agent %v", err)
		return err
	}
	err = addonManager.AddAgent(agentAddon)
	if err != nil {
		return err
	}
	return nil
}
