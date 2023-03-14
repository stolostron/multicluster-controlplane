// Copyright Contributors to the Open Cluster Management project

package managedclusteraddons

import (
	"context"
	"time"

	openshiftclientset "github.com/openshift/client-go/config/clientset/versioned"
	openshiftoauthclientset "github.com/openshift/client-go/oauth/clientset/versioned"
	clusterv1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	clusterclaimctl "github.com/stolostron/multicloud-operators-foundation/pkg/klusterlet/clusterclaim"
	clusterinfoctl "github.com/stolostron/multicloud-operators-foundation/pkg/klusterlet/clusterinfo"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterinformers "open-cluster-management.io/api/client/cluster/informers/externalversions"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

func InstallManagedClusterInfoAddon(ctx context.Context, o *agent.AgentOptions) error {
	hubConfig, err := clientcmd.BuildConfigFromFlags("", o.RegistrationAgent.BootstrapKubeconfig)
	if err != nil {
		return err
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))

	mgr, err := ctrl.NewManager(hubConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0",
	})
	if err != nil {
		klog.Errorf("unable to start managed cluster info manager %v", err)
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(o.KubeConfig)
	if err != nil {
		return err
	}

	managedClusterKubeClient, err := kubernetes.NewForConfig(o.KubeConfig)
	if err != nil {
		klog.Errorf("Unable to create managed cluster kube client due to %v", err)
		return err
	}

	managedClusterClusterClient, err := clusterclientset.NewForConfig(o.KubeConfig)
	if err != nil {
		klog.Errorf("Unable to create managed cluster cluster clientset due to %v", err)
		return err
	}

	openshiftClient, err := openshiftclientset.NewForConfig(o.KubeConfig)
	if err != nil {
		klog.Errorf("Unable to create managed cluster openshift config clientset due to %v", err)
		return err
	}

	osOauthClient, err := openshiftoauthclientset.NewForConfig(o.KubeConfig)
	if err != nil {
		klog.Errorf("Unable to create managed cluster openshift oauth clientset due to %v", err)
		return err
	}

	restMapper, err := apiutil.NewDynamicRESTMapper(o.KubeConfig, apiutil.WithLazyDiscovery)
	if err != nil {
		klog.Errorf("Unable to create restmapper due to %v", err)
		return err
	}

	kubeInformerFactory := informers.NewSharedInformerFactory(managedClusterKubeClient, 10*time.Minute)
	clusterInformerFactory := clusterinformers.NewSharedInformerFactory(managedClusterClusterClient, 10*time.Minute)

	clusterInfoReconciler := clusterinfoctl.ClusterInfoReconciler{
		Client:                   mgr.GetClient(),
		Log:                      ctrl.Log.WithName("controllers").WithName("ManagedClusterInfo"),
		Scheme:                   mgr.GetScheme(),
		NodeInformer:             kubeInformerFactory.Core().V1().Nodes(),
		ClaimInformer:            clusterInformerFactory.Cluster().V1alpha1().ClusterClaims(),
		ClaimLister:              clusterInformerFactory.Cluster().V1alpha1().ClusterClaims().Lister(),
		ManagedClusterClient:     managedClusterKubeClient,
		ClusterName:              o.RegistrationAgent.ClusterName,
		ConfigV1Client:           openshiftClient,
		DisableLoggingInfoSyncer: true,
	}

	clusterClaimer := clusterclaimctl.ClusterClaimer{
		ClusterName:                     o.RegistrationAgent.ClusterName,
		HubClient:                       mgr.GetClient(),
		KubeClient:                      managedClusterKubeClient,
		ConfigV1Client:                  openshiftClient,
		OauthV1Client:                   osOauthClient,
		Mapper:                          restMapper,
		EnableSyncLabelsToClusterClaims: true,
	}

	clusterClaimReconciler := clusterclaimctl.ClusterClaimReconciler{
		Log:               ctrl.Log.WithName("controllers").WithName("ManagedClusterInfo"),
		ClusterClient:     managedClusterClusterClient,
		ClusterInformers:  clusterInformerFactory.Cluster().V1alpha1().ClusterClaims(),
		ListClusterClaims: clusterClaimer.List,
	}

	waitForClusterClaimReady(ctx, dynamicClient)

	klog.Info("starting managedclusterinfo controller")
	if err := clusterInfoReconciler.SetupWithManager(mgr); err != nil {
		klog.Error(err)
		return err
	}

	klog.Info("starting clusterclaim controller")
	if err = clusterClaimReconciler.SetupWithManager(mgr); err != nil {
		klog.Error(err)
		return err
	}

	go kubeInformerFactory.Start(ctx.Done())
	go clusterInformerFactory.Start(ctx.Done())

	if err := mgr.Start(ctx); err != nil {
		klog.Error(err)
		return err
	}

	<-ctx.Done()

	return nil
}

func waitForClusterClaimReady(ctx context.Context, dynamicClient dynamic.Interface) bool {
	if err := wait.PollUntil(1*time.Second, func() (bool, error) {
		_, err := dynamicClient.Resource(schema.GroupVersionResource{
			Group:    apiextensionsv1.SchemeGroupVersion.Group,
			Version:  apiextensionsv1.SchemeGroupVersion.Version,
			Resource: "customresourcedefinitions",
		}).Get(ctx, "clusterclaims.cluster.open-cluster-management.io", metav1.GetOptions{})
		if err != nil {
			klog.Infof("waiting clusterclaim crd: %v", err)
			return false, nil
		}
		klog.Infof("clusterclaim crd is ready")
		return true, nil
	}, ctx.Done()); err != nil {
		return false
	}
	return true
}
