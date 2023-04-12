package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/cli"
	logsapi "k8s.io/component-base/logs/api/v1"
	ocmfeature "open-cluster-management.io/registration/pkg/features"

	"github.com/stolostron/multicluster-controlplane/cmd/agent"
	"github.com/stolostron/multicluster-controlplane/cmd/manager"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
)

func init() {
	utilruntime.Must(logsapi.AddFeatureGates(utilfeature.DefaultMutableFeatureGate)) // register log to featuregate
	utilruntime.Must(utilfeature.DefaultMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates))
	ocmfeature.DefaultSpokeMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates)
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
