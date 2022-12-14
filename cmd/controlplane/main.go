package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"

	"github.com/stolostron/multicluster-controlplane/pkg/options"
	"github.com/stolostron/multicluster-controlplane/pkg/servers"
)

func init() {
	utilruntime.Must(logsapi.AddFeatureGates(utilfeature.DefaultMutableFeatureGate)) // register log to featuregate
}

func main() {
	command := newControlPlaneCommand()
	code := cli.Run(command)
	os.Exit(code)
}

func newControlPlaneCommand() *cobra.Command {
	options := options.NewOptions()
	cmd := &cobra.Command{
		Use:   "controlplane",
		Short: "Multicluster Controlpane Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			fs := cmd.Flags()
			if err := logsapi.ValidateAndApply(options.Generic.Logs, utilfeature.DefaultFeatureGate); err != nil {
				return err
			}
			cliflag.PrintFlags(fs)

			stopChan := genericapiserver.SetupSignalHandler()
			completedOptions, err := options.CompletedAndValidateOptions(stopChan)
			if err != nil {
				return err
			}
			return servers.Run(completedOptions, stopChan)
		},
		Args: func(cmd *cobra.Command, args []string) error {
			for _, arg := range args {
				if len(arg) > 0 {
					return fmt.Errorf("%q does not take any arguments, got %q", cmd.CommandPath(), args)
				}
			}
			return nil
		},
	}
	fs := cmd.Flags()
	options.AddFlags(fs)
	return cmd
}
