package klusterlet

import (
	"context"

	"github.com/openshift/library-go/pkg/operator/events"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/klusterletcontroller"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet/controllers/statuscontroller"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"

	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	operatorv1informers "open-cluster-management.io/api/client/operator/informers/externalversions/operator/v1"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
)

func StartKlusterlet(ctx context.Context,
	controlplaneAPIExtensionClient apiextensionsclient.Interface,
	klusterletClient operatorv1client.KlusterletInterface,
	kubeClient kubernetes.Interface,
	workClient workclient.Interface,
	kubeInformerFactory informers.SharedInformerFactory,
	klusterletInformer operatorv1informers.KlusterletInformer,
) {
	recorder := events.NewInMemoryRecorder("klusterlet-controller")

	klusterletController := klusterletcontroller.NewKlusterletController(
		kubeClient,
		controlplaneAPIExtensionClient,
		klusterletClient,
		klusterletInformer,
		kubeInformerFactory.Core().V1().Secrets(),
		kubeInformerFactory.Apps().V1().Deployments(),
		workClient.WorkV1().AppliedManifestWorks(),
		recorder,
	)

	statusController := statuscontroller.NewKlusterletStatusController(
		kubeClient,
		klusterletClient,
		klusterletInformer,
		kubeInformerFactory.Apps().V1().Deployments(),
		recorder,
	)

	// TODO need go through this controller and do more test
	klusterletCleanupController := klusterletcontroller.NewKlusterletCleanupController(
		kubeClient,
		controlplaneAPIExtensionClient,
		klusterletClient,
		klusterletInformer,
		kubeInformerFactory.Core().V1().Secrets(),
		kubeInformerFactory.Apps().V1().Deployments(),
		workClient.WorkV1().AppliedManifestWorks(),
		recorder,
	)

	// TODO enable bootstrap controller?
	// TODO enable sar controller?

	go klusterletController.Run(ctx, 1)
	go klusterletCleanupController.Run(ctx, 1)
	go statusController.Run(ctx, 1)
}
