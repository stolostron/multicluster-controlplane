// Copyright Contributors to the Open Cluster Management project

package manager

import (
	"fmt"

	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	cliflag "k8s.io/component-base/cli/flag"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"
	"k8s.io/klog"
	ocmfeature "open-cluster-management.io/api/feature"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers/options"

	controller "github.com/stolostron/multicluster-controlplane/pkg/controllers"
)

func NewManager() *cobra.Command {
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
