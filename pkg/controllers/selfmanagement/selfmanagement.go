package selfmanagement

import (
	"context"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/controllers/ocmcontroller"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers/options"

	"github.com/stolostron/multicluster-controlplane/pkg/agent"
)

func InstallControllers(options *options.ServerRunOptions) func(<-chan struct{}, *aggregatorapiserver.Config) error {
	return func(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
		if _, err := rest.InClusterConfig(); err != nil {
			klog.Warning("Current runtime environment is not in a cluster, ignore --self-management flag.")
			return nil
		}

		if !options.EnableSelfManagement {
			return nil
		}

		go func() {
			ctx := ocmcontroller.GoContext(stopCh)

			hubRestConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
			hubRestConfig.ContentType = "application/json"

			clusterClient, err := clusterclient.NewForConfig(hubRestConfig)
			if err != nil {
				klog.Fatalf("failed to build cluster client, %v", err)
			}

			// wait for the agent is registered
			if err := waitForClusterAvailable(ctx, clusterClient, options.SelfManagementClusterName); err != nil {
				klog.Fatalf("failed to wait for self management cluster %s available, %v",
					options.SelfManagementClusterName, err)
			}

			// set required env
			if err := os.Setenv("OPERATOR_NAME", "multicluster-controlplane"); err != nil {
				klog.Fatalf("failed to set env `OPERATOR_NAME`, %v", err)
			}
			if err := os.Setenv("WATCH_NAMESPACE", options.SelfManagementClusterName); err != nil {
				klog.Fatalf("failed to set evn `WATCH_NAMESPACE`, %v", err)
			}

			agentOptions := agent.NewAgentOptions().
				WithHubKubeConfig(hubRestConfig).
				WithClusterName(options.SelfManagementClusterName)

			klog.Info("starting addon agents")
			if err := agentOptions.RunAddOns(ctx); err != nil {
				klog.Fatalf("failed to start addon agents for self management, %v", err)
			}

			<-ctx.Done()
		}()

		return nil
	}
}

func waitForClusterAvailable(ctx context.Context, clusterClient clusterclient.Interface, name string) error {
	return wait.PollImmediate(5*time.Second, 10*time.Minute, func() (bool, error) {
		cluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return false, nil
		}

		if err != nil {
			return false, err
		}

		return meta.IsStatusConditionTrue(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable), nil
	})
}
