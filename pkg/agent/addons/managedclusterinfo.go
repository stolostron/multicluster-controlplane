package addons

import (
	"context"
	"time"

	openshiftclientset "github.com/openshift/client-go/config/clientset/versioned"
	openshiftoauthclientset "github.com/openshift/client-go/oauth/clientset/versioned"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	clusterclaimctl "github.com/stolostron/multicluster-controlplane/pkg/agent/addons/controllers/clusterclaim"
	clusterinfoctl "github.com/stolostron/multicluster-controlplane/pkg/agent/addons/controllers/clusterinfo"
)

func StartManagedClusterInfoAgent(
	ctx context.Context,
	clusterName string,
	mgr manager.Manager,
	kubeClient kubernetes.Interface,
	dynamicClient dynamic.Interface,
	clusterClient clusterclientset.Interface,
	ocpClient openshiftclientset.Interface,
	ocpOauthClient openshiftoauthclientset.Interface,
	restMapper meta.RESTMapper) error {
	kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(clusterClient, 10*time.Minute)

	clusterInfoReconciler := clusterinfoctl.ClusterInfoReconciler{
		Client:               mgr.GetClient(),
		Log:                  ctrl.Log.WithName("controllers").WithName("ManagedClusterInfo"),
		Scheme:               mgr.GetScheme(),
		NodeInformer:         kubeInformerFactory.Core().V1().Nodes(),
		ClaimInformer:        clusterInformerFactory.Cluster().V1alpha1().ClusterClaims(),
		ClaimLister:          clusterInformerFactory.Cluster().V1alpha1().ClusterClaims().Lister(),
		ManagedClusterClient: kubeClient,
		ClusterName:          clusterName,
		ConfigV1Client:       ocpClient,
	}

	clusterClaimer := clusterclaimctl.ClusterClaimer{
		ClusterName:                     clusterName,
		HubClient:                       mgr.GetClient(),
		KubeClient:                      kubeClient,
		ConfigV1Client:                  ocpClient,
		OauthV1Client:                   ocpOauthClient,
		Mapper:                          restMapper,
		EnableSyncLabelsToClusterClaims: true,
	}

	clusterClaimReconciler := clusterclaimctl.ClusterClaimReconciler{
		Log:               ctrl.Log.WithName("controllers").WithName("ManagedClusterInfo"),
		ClusterClient:     clusterClient,
		ClusterInformers:  clusterInformerFactory.Cluster().V1alpha1().ClusterClaims(),
		ListClusterClaims: clusterClaimer.List,
	}

	klog.Info("starting managedclusterinfo controller")
	if err := clusterInfoReconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	klog.Info("starting clusterclaim controller")
	if err := clusterClaimReconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	go kubeInformerFactory.Start(ctx.Done())
	go clusterInformerFactory.Start(ctx.Done())

	return nil
}
