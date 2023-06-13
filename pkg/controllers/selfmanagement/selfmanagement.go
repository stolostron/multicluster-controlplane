package selfmanagement

import (
	"context"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/features"
	"open-cluster-management.io/multicluster-controlplane/pkg/servers/options"
	"open-cluster-management.io/multicluster-controlplane/pkg/util"

	"github.com/stolostron/multicluster-controlplane/pkg/agent"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
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
			ctx := util.GoContext(stopCh)

			hubRestConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
			hubRestConfig.ContentType = "application/json"

			clusterClient, err := clusterclient.NewForConfig(hubRestConfig)
			if err != nil {
				klog.Fatalf("failed to build cluster client, %v", err)
			}

			// wait for the agent is registered
			clusterName, err := waitForClusterAvailable(ctx, clusterClient, options.SelfManagementClusterName)
			if err != nil {
				klog.Fatalf("failed to wait for self management cluster available, %v", err)
			}

			// set required env
			if err := os.Setenv("OPERATOR_NAME", "multicluster-controlplane"); err != nil {
				klog.Fatalf("failed to set env `OPERATOR_NAME`, %v", err)
			}
			if err := os.Setenv("WATCH_NAMESPACE", clusterName); err != nil {
				klog.Fatalf("failed to set evn `WATCH_NAMESPACE`, %v", err)
			}

			// set agent feature gates
			if err := features.DefaultAgentMutableFeatureGate.Add(feature.DefaultControlPlaneFeatureGates); err != nil {
				klog.Fatalf("failed to set agent feature gates, %v", err)
			}

			agentOptions := agent.NewAgentOptions().
				WithHubKubeConfig(hubRestConfig).
				WithClusterName(clusterName).
				WithSelfManagementEnabled(true)

			klog.Info("starting addon agents")
			if err := agentOptions.RunAddOns(ctx); err != nil {
				klog.Fatalf("failed to start addon agents for self management, %v", err)
			}

			<-ctx.Done()
		}()

		return nil
	}
}

func waitForClusterAvailable(ctx context.Context, clusterClient clusterclient.Interface, name string) (string, error) {
	var clusterName string
	if err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		clusters, err := clusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{
			LabelSelector: "multicluster-controlplane.open-cluster-management.io/selfmanagement",
		})

		if err != nil {
			return false, err
		}

		if len(clusters.Items) != 1 {
			return false, nil
		}

		cluster := clusters.Items[0]
		clusterName = cluster.Name
		return meta.IsStatusConditionTrue(cluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable), nil
	}); err != nil {
		return "", err
	}

	return clusterName, nil
}
