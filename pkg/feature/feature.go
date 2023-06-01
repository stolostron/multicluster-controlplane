package feature

import (
	"k8s.io/component-base/featuregate"
)

const (

	// ConfigurationPolicy will start new controllers in the controlplane agent process to manage the lifecycle of configuration policy.
	ConfigurationPolicy featuregate.Feature = "ConfigurationPolicy"

	// ManagedClusterInfo will start new controllers in the controlplane agent process to manage the managed cluster info in cluster namespace.
	// It depends on the ClusterClaim feature.
	ManagedClusterInfo featuregate.Feature = "ManagedClusterInfo"
)

var DefaultControlPlaneFeatureGates = map[featuregate.Feature]featuregate.FeatureSpec{
	ConfigurationPolicy: {Default: true, PreRelease: featuregate.Alpha},
	ManagedClusterInfo:  {Default: true, PreRelease: featuregate.Alpha},
}
