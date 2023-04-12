// Copyright Contributors to the Open Cluster Management project
package constants

const (
	DefaultNamespace                  = "default"
	DefaultAddOnAgentInstallNamespace = "open-cluster-management-agent-addon"
)

// addon image info
const (
	SnapshotVersionEnvName = "SNAPSHOT_VERSION"
	DefaultSnapshotVersion = "2.7.0-SNAPSHOT-2023-02-08-22-11-25"

	MultiCloudManagerImageEnvName = "MULTICLOUD_MANAGER_IMAGE"
	DefaultMultiCloudManagerImage = "quay.io/stolostron/multicloud-manager"

	ManagedServiceAccountImageEnvName = "MANAGED_SERVICE_ACCOUNT_IMAGE"
	DefaultManagedServiceAccountImage = "quay.io/stolostron/managed-serviceaccount"

	GovernancePolicyFrameworkAddonImageEnvName = "GOVERNANCE_POLICY_FRAMEWORK_ADDON_IMAGE"
	DefaultGovernancePolicyFrameworkAddonImage = "quay.io/stolostron/governance-policy-framework-addon"

	ConfigPolicyControllerImageEnvName = "CONFIG_POLICY_CONTROLLER_IMAGE"
	DefaultConfigPolicyControllerImage = "quay.io/stolostron/config-policy-controller"

	KubeRBACProxyEnvName      = "KUBE_RBAC_PROXY_IMAGE"
	DefaultKubeRBACProxyImage = "quay.io/stolostron/kube-rbac-proxy"
)
