// Copyright Contributors to the Open Cluster Management project

package agent

import (
	"context"

	"github.com/spf13/cobra"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/klog"

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

			// TODO change the function `waitForValidHubKubeConfig` to publich in
			// `open-cluster-management.io/multicluster-controlplane/pkg/agent`, then call here

			if err := agentOptions.RunAddOns(ctx); err != nil {
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
