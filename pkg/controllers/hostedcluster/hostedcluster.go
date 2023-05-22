package hostedcluster

import (
	hyperv1beta1 "github.com/openshift/hypershift/api/v1beta1"

	// _ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	"open-cluster-management.io/multicluster-controlplane/pkg/controllers/ocmcontroller"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/hostedcluster/controller"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(hyperv1beta1.AddToScheme(scheme))
}

// InstallControllers installs next-gen controlplane controllers in hub cluster
func InstallControllers(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	ctx := ocmcontroller.GoContext(stopCh)

	loopbackRestConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
	loopbackRestConfig.ContentType = "application/json"
	controlplaneKubeClient, err := kubernetes.NewForConfig(loopbackRestConfig)
	if err != nil {
		return err
	}
	controlplaneOperatorClient, err := operatorclient.NewForConfig(loopbackRestConfig)
	if err != nil {
		return err
	}

	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	go func() {
		mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: "0", //TODO think about the mertics later
		})
		if err != nil {
			klog.Fatalf("unable to start manager %v", err)
		}

		hostedClusterReconciler := controller.HostedClusterReconciler{
			Client:                     mgr.GetClient(),
			ControlplaneKubeClient:     controlplaneKubeClient,
			ControlplaneOperatorClient: controlplaneOperatorClient,
		}

		klog.Info("add hostedcluster controller to manager")
		if err := hostedClusterReconciler.SetupWithManager(mgr); err != nil {
			klog.Fatalf("failed to setup hostedcluster controller %v", err)
		}

		if err := mgr.Start(ctx); err != nil {
			klog.Fatalf("failed to start controller manager, %v", err)
		}

		<-ctx.Done()
	}()

	return nil
}
