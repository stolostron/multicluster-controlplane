package feature

import "k8s.io/component-base/featuregate"

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

var DefaultControlPlaneFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	ConfigurationPolicy:   {Default: true, PreRelease: featuregate.Alpha},
	ManagedClusterInfo:    {Default: true, PreRelease: featuregate.Alpha},
	ManagedServiceAccount: {Default: false, PreRelease: featuregate.Alpha},
}
