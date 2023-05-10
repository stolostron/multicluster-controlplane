// Copyright Contributors to the Open Cluster Management project

package agent

import (
	"context"
	"path"

	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stolostron/multicluster-controlplane/pkg/agent"
)

func NewAgent() *cobra.Command {
	agentOptions := agent.NewAgentOptions()
	var logLevel string
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start a Multicluster Controlplane Agent",
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
		true,
		"Disable custom metrics collection",
	)

	flags.StringVar(
		&logLevel,
		"log-level",
		"info",
		"Zap level to configure the verbosity of logging.",
	)

	agentOptions.AddFlags(flags)
	return cmd
}
