package addons

import (
	"context"
	"time"

	openshiftclientset "github.com/openshift/client-go/config/clientset/versioned"
	openshiftoauthclientset "github.com/openshift/client-go/oauth/clientset/versioned"
	"github.com/openshift/library-go/pkg/controller/factory"

	clusterinfoclient "github.com/stolostron/cluster-lifecycle-api/client/clusterinfo/clientset/versioned"
	clusterinfoinformers "github.com/stolostron/cluster-lifecycle-api/client/clusterinfo/informers/externalversions"
	clusterinfoinformer "github.com/stolostron/cluster-lifecycle-api/client/clusterinfo/informers/externalversions/clusterinfo/v1beta1"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1alpha1informer "open-cluster-management.io/api/client/cluster/informers/externalversions/cluster/v1alpha1"
	"open-cluster-management.io/multicluster-controlplane/pkg/util"

	"github.com/stolostron/multicluster-controlplane/pkg/agent/addons/controllers/clusterclaim"
	"github.com/stolostron/multicluster-controlplane/pkg/agent/addons/controllers/clusterinfo"
)

func StartManagedClusterInfoAgent(
	ctx context.Context,
	clusterName string,
	selfManagementEnabled bool,
	hubKubeConfig, spokeKubeConfig *rest.Config,
	restMapper meta.RESTMapper,
	kubeInformerFactory informers.SharedInformerFactory,
	clusterInformerFactory clusterinformers.SharedInformerFactory,
) error {
	clusterInfoClient, err := clusterinfoclient.NewForConfig(hubKubeConfig)
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	clusterClient, err := clusterclientset.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	ocpClient, err := openshiftclientset.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	ocpOauthClient, err := openshiftoauthclientset.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	clusterInfoInformerFactory := clusterinfoinformers.NewSharedInformerFactoryWithOptions(
		clusterInfoClient,
		10*time.Minute,
		clusterinfoinformers.WithNamespace(clusterName),
	)

	if selfManagementEnabled {
		httpClient, err := rest.HTTPClientFor(spokeKubeConfig)
		if err != nil {
			return err
		}

		restMapper, err = apiutil.NewDynamicRESTMapper(spokeKubeConfig, httpClient)
		if err != nil {
			return err
		}

		kubeInformerFactory = informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
		clusterInformerFactory = clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)
	}

	clusterInfoInformer := clusterInfoInformerFactory.Internal().V1beta1().ManagedClusterInfos()
	nodeInformer := kubeInformerFactory.Core().V1().Nodes()
	claimInformer := clusterInformerFactory.Cluster().V1alpha1().ClusterClaims()

	clusterInfoReconciler := &clusterinfo.ClusterInfoReconciler{
		ManagedClusterInfoClient: clusterInfoClient,
		ManagedClusterClient:     kubeClient,
		ConfigV1Client:           ocpClient,
		ClaimLister:              claimInformer.Lister(),
		ManagedClusterInfoList:   clusterInfoInformer.Lister(),
		ClusterName:              clusterName,
	}

	clusterClaimer := clusterclaim.ClusterClaimer{
		ClusterName:                     clusterName,
		KubeClient:                      kubeClient,
		ConfigV1Client:                  ocpClient,
		OauthV1Client:                   ocpOauthClient,
		ManagedClusterInfoList:          clusterInfoInformer.Lister(),
		Mapper:                          restMapper,
		EnableSyncLabelsToClusterClaims: true,
	}

	clusterClaimReconciler := &clusterclaim.ClusterClaimReconciler{
		ClusterClient:     clusterClient,
		ListClusterClaims: clusterClaimer.List,
	}

	workmgrController := newWorkMgrController(
		clusterInfoReconciler,
		clusterClaimReconciler,
		clusterInfoInformer,
		nodeInformer,
		claimInformer,
	)

	go clusterInfoInformerFactory.Start(ctx.Done())

	if selfManagementEnabled {
		go kubeInformerFactory.Start(ctx.Done())
		go clusterInformerFactory.Start(ctx.Done())
	}

	go workmgrController.Run(ctx, 1)

	return nil
}

type workmgrController struct {
	clusterInfoReconciler  *clusterinfo.ClusterInfoReconciler
	clusterClaimReconciler *clusterclaim.ClusterClaimReconciler
}

func newWorkMgrController(
	clusterInfoReconciler *clusterinfo.ClusterInfoReconciler,
	clusterClaimReconciler *clusterclaim.ClusterClaimReconciler,
	clusterInfoInformer clusterinfoinformer.ManagedClusterInfoInformer,
	nodeInformer coreinformers.NodeInformer,
	claimInformer clusterv1alpha1informer.ClusterClaimInformer) factory.Controller {
	controller := &workmgrController{
		clusterInfoReconciler:  clusterInfoReconciler,
		clusterClaimReconciler: clusterClaimReconciler,
	}
	return factory.New().WithSync(controller.sync).
		WithInformers(nodeInformer.Informer(), clusterInfoInformer.Informer(), claimInformer.Informer()).
		ToController("WorkerManagerController", util.NewLoggingRecorder("workmgr-controller"))
}

func (c *workmgrController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	errs := []error{}
	if _, err := c.clusterInfoReconciler.Reconcile(ctx, reconcile.Request{}); err != nil {
		errs = append(errs, err)
	}
	if _, err := c.clusterClaimReconciler.Reconcile(ctx, reconcile.Request{}); err != nil {
		errs = append(errs, err)
	}

	return errors.NewAggregate(errs)
}
