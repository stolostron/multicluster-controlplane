// Copyright Contributors to the Open Cluster Management project

package agent

import (
	"context"
	"path"

	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/klog"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"

	"github.com/stolostron/multicluster-controlplane/pkg/agent"
)

func NewAgent() *cobra.Command {
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
	flags.StringVar((*string)(&agentOptions.DeployMode), "deploy-mode", string(operatorapiv1.InstallModeDefault),
		"Indicate the deploy mode of the agent, Default or Hosted")
	agentOptions.AddFlags(flags)
	return cmd
}
