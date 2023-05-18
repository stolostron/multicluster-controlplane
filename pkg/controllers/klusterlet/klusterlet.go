package klusterlet

import (
	"context"

	"github.com/openshift/library-go/pkg/controller/factory"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	operatorv1informers "open-cluster-management.io/api/client/operator/informers/externalversions/operator/v1"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	workv1client "open-cluster-management.io/api/client/work/clientset/versioned/typed/work/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/util"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/bootstrapcontroller"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/klusterletcontroller"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/ssarcontroller"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/statuscontroller"
)

type Klusterlet struct {
	klusterletController factory.Controller
	cleanupController    factory.Controller
	statusController     factory.Controller
	ssarController       factory.Controller
	bootstrapController  factory.Controller
}

func (k *Klusterlet) Start(ctx context.Context) {
	go k.klusterletController.Run(ctx, 1)
	go k.cleanupController.Run(ctx, 1)
	go k.statusController.Run(ctx, 1)
	go k.ssarController.Run(ctx, 1)
	go k.bootstrapController.Run(ctx, 1)
}

func NewKlusterlet(
	controlplaneKubeClient kubernetes.Interface,
	controlplaneDynamicClient dynamic.Interface,
	controlplaneClusterClient clusterclient.Interface,
	controlplaneWorkClient workclient.Interface,
	controlplaneAPIExtensionClient apiextensionsclient.Interface,
	klusterletClient operatorv1client.KlusterletInterface,
	kubeClient kubernetes.Interface,
	appliedManifestWorkClient workv1client.AppliedManifestWorkInterface,
	kubeInformerFactory informers.SharedInformerFactory,
	klusterletInformer operatorv1informers.KlusterletInformer,
) *Klusterlet {
	recorder := util.NewLoggingRecorder("klusterlet-controller")
	return &Klusterlet{
		klusterletController: klusterletcontroller.NewKlusterletController(
			kubeClient,
			controlplaneKubeClient,
			controlplaneAPIExtensionClient,
			klusterletClient,
			klusterletInformer,
			kubeInformerFactory.Core().V1().Secrets(),
			kubeInformerFactory.Apps().V1().Deployments(),
			appliedManifestWorkClient,
			recorder,
		),
		cleanupController: klusterletcontroller.NewKlusterletCleanupController(
			kubeClient,
			controlplaneKubeClient,
			controlplaneDynamicClient,
			controlplaneClusterClient,
			controlplaneWorkClient,
			controlplaneAPIExtensionClient,
			klusterletClient,
			klusterletInformer,
			kubeInformerFactory.Core().V1().Secrets(),
			kubeInformerFactory.Apps().V1().Deployments(),
			appliedManifestWorkClient,
			recorder,
		),
		statusController: statuscontroller.NewKlusterletStatusController(
			kubeClient,
			klusterletClient,
			klusterletInformer,
			kubeInformerFactory.Apps().V1().Deployments(),
			recorder,
		),
		ssarController: ssarcontroller.NewKlusterletSSARController(
			kubeClient,
			klusterletClient,
			klusterletInformer,
			kubeInformerFactory.Core().V1().Secrets(),
			recorder,
		),
		bootstrapController: bootstrapcontroller.NewBootstrapController(
			kubeClient,
			klusterletInformer,
			kubeInformerFactory.Core().V1().Secrets(),
			recorder,
		),
	}
}
