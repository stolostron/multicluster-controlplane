// Copyright Contributors to the Open Cluster Management project

package main

import (
	"os"

	"github.com/spf13/cobra"

	"go.uber.org/zap/zapcore"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/component-base/cli"
	cliflag "k8s.io/component-base/cli/flag"
	logsapi "k8s.io/component-base/logs/api/v1"
	"k8s.io/component-base/version/verflag"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"open-cluster-management.io/multicluster-controlplane/pkg/features"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers/options"

	controller "github.com/stolostron/multicluster-controlplane/pkg/controllers"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/selfmanagement"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
)

var logLevel string

func init() {
	// register log to featuregate
	utilruntime.Must(logsapi.AddFeatureGates(utilfeature.DefaultMutableFeatureGate))
	// init feature gates
	utilruntime.Must(features.DefaultControlplaneMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates))
}

func main() {
	options := options.NewServerRunOptions()
	cmd := &cobra.Command{
		Use:   "multicluster-controlplane",
		Short: "Start a multicluster controlplane",
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()
			cliflag.PrintFlags(cmd.Flags())

			level, _ := zapcore.ParseLevel(logLevel)
			opts := &zap.Options{
				Level: zapcore.Level(level),
			}
			// set log level to the controller-runtime logger
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts)))

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
			server.AddController("next-gen-controlplane-controllers", controller.InstallControllers)
			server.AddController("next-gen-controlplane-self-management", selfmanagement.InstallControllers(options))

			return server.Start(stopChan)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(
		&logLevel,
		"log-level",
		"info",
		"Zap level to configure the verbosity of logging.",
	)
	options.AddFlags(flags)

	os.Exit(cli.Run(cmd))
}
