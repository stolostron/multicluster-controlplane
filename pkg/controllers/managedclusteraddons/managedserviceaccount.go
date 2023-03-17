// Copyright Contributors to the Open Cluster Management project

package managedclusteraddons

import (
	"k8s.io/client-go/kubernetes"
	"open-cluster-management.io/addon-framework/pkg/addonfactory"
	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	"open-cluster-management.io/addon-framework/pkg/agent"
	"open-cluster-management.io/api/client/addon/clientset/versioned"
	"open-cluster-management.io/managed-serviceaccount/pkg/addon/manager"
	"open-cluster-management.io/managed-serviceaccount/pkg/common"

	"github.com/stolostron/multicluster-controlplane/pkg/constants"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/helpers"
)

func AddManagedServiceAccountAddon(addonManager addonmanager.AddonManager, kubeClient kubernetes.Interface, addonClient versioned.Interface) error {

	agentAddOn, err := addonfactory.NewAgentAddonFactory(common.AddonName, manager.FS, "manifests/templates").
		WithConfigGVRs(addonfactory.AddOnDeploymentConfigGVR).
		WithGetValuesFuncs(
			manager.GetDefaultValues(helpers.GetImage(
				constants.ManagedServiceAccountImageEnvName,
				constants.DefaultManagedServiceAccountImage,
			), nil),
			addonfactory.GetAddOnDeloymentConfigValues(
				addonfactory.NewAddOnDeloymentConfigGetter(addonClient),
				addonfactory.ToAddOnDeloymentConfigValues,
			),
		).
		WithAgentRegistrationOption(manager.NewRegistrationOption(kubeClient)).
		WithInstallStrategy(agent.InstallAllStrategy(addonfactory.AddonDefaultInstallNamespace)).
		BuildTemplateAgentAddon()
	if err != nil {
		return err
	}

	// add agentaddon to addonmanager
	if err := addonManager.AddAgent(agentAddOn); err != nil {
		return err
	}

	return nil

}
