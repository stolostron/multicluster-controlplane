package config

import (
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
)

type AgentOptions struct {
	agent.AgentOptions
	DecryptionConcurrency uint8
	EvaluationConcurrency uint8
	EnableMetrics         bool
	Frequency             uint
}
