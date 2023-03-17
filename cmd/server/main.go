package main

import (
	"context"
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
	"k8s.io/klog"
	ocmfeature "open-cluster-management.io/api/feature"
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers/options"

	controller "github.com/stolostron/multicluster-controlplane/pkg/controllers"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/managedclusteraddons"
)

func init() {
	utilruntime.Must(logsapi.AddFeatureGates(utilfeature.DefaultMutableFeatureGate)) // register log to featuregate
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

	cmd.AddCommand(newManager())
	cmd.AddCommand(newAgent())

	return cmd
}

func newManager() *cobra.Command {
	options := options.NewServerRunOptions()
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start a Multicluster Controlplane Server",
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()
			cliflag.PrintFlags(cmd.Flags())

			if err := logsapi.ValidateAndApply(options.Logs, utilfeature.DefaultFeatureGate); err != nil {
				return err
			}

			stopChan := genericapiserver.SetupSignalHandler()
			if err := options.Complete(stopChan); err != nil {
				return err
			}

			if err := options.Validate(); err != nil {
				return err
			}

			server := servers.NewServer(*options)

			if utilfeature.DefaultMutableFeatureGate.Enabled(ocmfeature.AddonManagement) {
				klog.Info("enabled addons")
				server.AddController("multicluster-controlplane-ocm-addon-crd", controller.InstallAddonCrds)
				server.AddController("multicluster-controlplane-cluster-management-addons", controller.InstallClusterManagementAddons)
				server.AddController("multicluster-controlplane-managed-cluster-addons", controller.InstallManagedClusterAddons)
			}

			return server.Start(stopChan)
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
	options.AddFlags(cmd.Flags())
	return cmd
}

func newAgent() *cobra.Command {
	agentOptions := agent.NewAgentOptions()

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start a Multicluster Controlplane Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			shutdownCtx, cancel := context.WithCancel(context.TODO())

			shutdownHandler := genericapiserver.SetupSignalHandler()
			go func() {
				defer cancel()
				<-shutdownHandler
				klog.Infof("Received SIGTERM or SIGINT signal, shutting down agent.")
			}()

			ctx, terminate := context.WithCancel(shutdownCtx)
			defer terminate()

			// start managed cluster info controller
			go managedclusteraddons.InstallManagedClusterInfoAddon(ctx, agentOptions)

			if err := agentOptions.RunAgent(ctx); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		},
	}

	flags := cmd.Flags()
	agentOptions.AddFlags(flags)
	return cmd
}
