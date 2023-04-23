package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/component-base/cli"

	"open-cluster-management.io/multicluster-controlplane/pkg/features"

	"github.com/stolostron/multicluster-controlplane/cmd/agent"
	"github.com/stolostron/multicluster-controlplane/cmd/manager"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
)

func init() {
	utilruntime.Must(features.DefaultControlplaneMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates))
	utilruntime.Must(features.DefaultAgentMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates))
}

func main() {
	command := newControlPlaneCommand()
	os.Exit(cli.Run(command))
}

func newControlPlaneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "controlplane",
		Short: "Start a multicluster controlplane",
		Run: func(cmd *cobra.Command, args []string) {
			if err := cmd.Help(); err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
			}
			os.Exit(1)
		},
	}

	cmd.AddCommand(manager.NewManager())
	cmd.AddCommand(agent.NewAgent())

	return cmd
}
