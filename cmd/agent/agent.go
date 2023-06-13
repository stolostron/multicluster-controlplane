// Copyright Contributors to the Open Cluster Management project

package main

import (
	"context"
	"os"
	"path"

	"github.com/spf13/cobra"

	"go.uber.org/zap/zapcore"

	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"open-cluster-management.io/multicluster-controlplane/pkg/features"

	"github.com/stolostron/multicluster-controlplane/pkg/agent"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
)

var logLevel string

func init() {
	utilruntime.Must(features.DefaultAgentMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates))
}

func main() {
	agentOptions := agent.NewAgentOptions()
	cmd := &cobra.Command{
		Use:   "multicluster-agent",
		Short: "Start a multicluster agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			shutdownCtx, cancel := context.WithCancel(context.TODO())

			level, _ := zapcore.ParseLevel(logLevel)
			opts := &zap.Options{
				Level: zapcore.Level(level),
			}
			// set log level to the controller-runtime logger
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts)))

			shutdownHandler := genericapiserver.SetupSignalHandler()
			go func() {
				defer cancel()
				<-shutdownHandler
				klog.Infof("Received SIGTERM or SIGINT signal, shutting down agent.")
			}()

			ctx, terminate := context.WithCancel(shutdownCtx)
			defer terminate()

			// starting agent firstly to request the hub kubeconfig
			go func() {
				klog.Info("starting the controlplane agent")
				if err := agentOptions.RunAgent(ctx); err != nil {
					klog.Fatalf("failed to run agent, %v", err)
				}
			}()

			// wait for the agent is registered
			hubKubeConfig := path.Join(agentOptions.RegistrationAgent.HubKubeconfigDir, "kubeconfig")
			if err := agentOptions.WaitForValidHubKubeConfig(ctx, hubKubeConfig); err != nil {
				return err
			}

			if err := agentOptions.RunAddOns(ctx); err != nil {
				return err
			}

			<-ctx.Done()
			return nil
		},
	}

	flags := cmd.Flags()

	flags.UintVar(
		&agentOptions.Frequency,
		"update-frequency",
		10,
		"The status update frequency (in seconds) of a mutation policy",
	)

	flags.Uint8Var(
		&agentOptions.DecryptionConcurrency,
		"decryption-concurrency",
		5,
		"The max number of concurrent policy template decryptions",
	)

	flags.Uint8Var(
		&agentOptions.EvaluationConcurrency,
		"evaluation-concurrency",
		// Set a low default to not add too much load to the Kubernetes API server in resource constrained deployments.
		2,
		"The max number of concurrent configuration policy evaluations",
	)

	flags.BoolVar(
		&agentOptions.EnableMetrics,
		"enable-metrics",
		false,
		"Disable custom metrics collection",
	)

	flags.StringVar(
		&logLevel,
		"log-level",
		"info",
		"Zap level to configure the verbosity of logging.",
	)

	agentOptions.AddFlags(flags)

	os.Exit(cli.Run(cmd))
}
