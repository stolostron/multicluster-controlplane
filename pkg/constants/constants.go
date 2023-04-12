// Copyright Contributors to the Open Cluster Management project
package constants

import "k8s.io/component-base/featuregate"

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

const (

	// ConfigurationPolicy will start new controllers in the controlplane agent process to manage the lifecycle of configuration policy.
	ConfigurationPolicy featuregate.Feature = "ConfigurationPolicy"

	// ManagedClusterInfo will start new controllers in the controlplane agent process to manage the managed cluster info in cluster namespace.
	// It depends on the ClusterClaim feature.
	ManagedClusterInfo featuregate.Feature = "ManagedClusterInfo"

	// ManagedServiceAccount will start new controllers in the controlplane agent process to synchronize ServiceAccount to the managed clusters
	// and collecting the tokens from these local service accounts as secret resources back to the hub cluster.
	ManagedServiceAccount featuregate.Feature = "ManagedServiceAccount"
)
