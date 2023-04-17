package controller

import (
	_ "net/http/pprof"
	"time"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"
	"k8s.io/klog/v2/klogr"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"

	"open-cluster-management.io/addon-framework/pkg/addonmanager"
	addonclient "open-cluster-management.io/api/client/addon/clientset/versioned"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	operatorinformer "open-cluster-management.io/api/client/operator/informers/externalversions"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policyv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	authv1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/controllers/ocmcontroller"

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/addons"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/clusterimport"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/manifests"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
	"github.com/stolostron/multicluster-controlplane/pkg/helpers"
)

var requiredCRDs = []string{
	"crds/apps.open-cluster-management.io_placementrules.crd.yaml",
	"crds/authentication.open-cluster-management.io_managedserviceaccounts.crd.yaml",
	"crds/internal.open-cluster-management.io_managedclusterinfos.crd.yaml",
	"crds/policy.open-cluster-management.io_placementbindings.crd.yaml",
	"crds/operator.open-cluster-management.io_klusterlets.crd.yaml",
	"crds/policy.open-cluster-management.io_policies.crd.yaml",
	"crds/policy.open-cluster-management.io_policyautomations.crd.yaml",
	"crds/policy.open-cluster-management.io_policysets.crd.yaml",
}

var ResyncInterval = 5 * time.Minute

// InstallControllers installs next-gen controlplane controllers in hub cluster
func InstallControllers(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	ctx := ocmcontroller.GoContext(stopCh)
	loopbackRestConfig := aggregatorConfig.GenericConfig.LoopbackClientConfig
	loopbackRestConfig.ContentType = "application/json"

	controlplaneKubeClient, err := kubernetes.NewForConfig(loopbackRestConfig)
	if err != nil {
		return err
	}

	controlplaneDynamicClient, err := dynamic.NewForConfig(loopbackRestConfig)
	if err != nil {
		return err
	}

	controlplaneCRDClient, err := apiextensionsclient.NewForConfig(loopbackRestConfig)
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

	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	workClient, err := workclient.NewForConfig(restConfig)
	if err != nil {
		return err
	}

	if err := helpers.EnsureCRDs(ctx, controlplaneCRDClient, manifests.CRDFiles, requiredCRDs...); err != nil {
		return err
	}

	go func() {
		scheme := runtime.NewScheme()

		utilruntime.Must(kubescheme.AddToScheme(scheme))
		utilruntime.Must(authv1alpha1.AddToScheme(scheme))
		utilruntime.Must(clusterv1.AddToScheme(scheme))
		utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
		utilruntime.Must(policyv1.AddToScheme(scheme))
		utilruntime.Must(policyv1beta1.AddToScheme(scheme))
		utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))
		utilruntime.Must(placementrulev1.AddToScheme(scheme))

		kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, ResyncInterval)
		operatorInformerFactory := operatorinformer.NewSharedInformerFactory(controlplaneOperatorClient, ResyncInterval)

		secretInformer := corev1informers.NewFilteredSecretInformer(
			controlplaneKubeClient,
			metav1.NamespaceAll,
			ResyncInterval,
			cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc},
			func(listOptions *metav1.ListOptions) {
				listOptions.FieldSelector = fields.OneTermEqualSelector(
					"metadata.name", clusterimport.ManagedClusterKubeConfigSecretName).String()
			},
		)

		ctrl.SetLogger(klogr.New())
		mgr, err := ctrl.NewManager(loopbackRestConfig, ctrl.Options{
			Scheme: scheme,
		})
		if err != nil {
			klog.Fatalf("unable to start manager %v", err)
		}

		if utilfeature.DefaultMutableFeatureGate.Enabled(feature.ManagedClusterInfo) {
			klog.Info("starting managed cluster info addon")
			if err := addons.SetupManagedClusterInfoWithManager(ctx, mgr); err != nil {
				klog.Fatalf("failed to setup managedclusterinfo controller %v", err)
			}
		}

		if utilfeature.DefaultMutableFeatureGate.Enabled(feature.ConfigurationPolicy) {
			klog.Info("starting policy addon")
			if err := addons.SetupPolicyWithManager(
				ctx, mgr, restConfig, kubeClient, controlplaneDynamicClient); err != nil {
				klog.Fatalf("failed to setup policy controller %v", err)
			}
		}

		if utilfeature.DefaultMutableFeatureGate.Enabled(feature.ManagedServiceAccount) {
			klog.Info("starting managed serviceaccount addon")
			// TODO remove this if putting the managedserviceaccountaddon in-process
			addonManager, err := addonmanager.New(loopbackRestConfig)
			if err != nil {
				klog.Fatalf("failed to create addon manager %v", err)
			}
			addonClient, err := addonclient.NewForConfig(loopbackRestConfig)
			if err != nil {
				klog.Fatalf("failed to create addon client %v", err)
			}
			if err := addons.SetupManagedServiceAccountWithManager(
				ctx, mgr, addonManager, controlplaneKubeClient, addonClient); err != nil {
				klog.Fatalf("failed to setup managedserviceaccount controller %v", err)
			}

			go addonManager.Start(ctx)
		}

		klog.Info("starting cluster import controller")
		if err := clusterimport.SetupClusterImportController(
			mgr,
			kubeClient,
			controlplaneOperatorClient.OperatorV1().Klusterlets(),
			secretInformer,
			operatorInformerFactory.Operator().V1().Klusterlets().Informer(),
		); err != nil {
			klog.Fatalf("failed to setup managedserviceaccount manager %v", err)
		}

		// TODO remove this after using latest registration agent
		if err := clusterimport.SetupCSRApprovalController(mgr, controlplaneKubeClient); err != nil {
			klog.Fatalf("failed to setup managedserviceaccount manager %v", err)
		}

		go kubeInformerFactory.Start(ctx.Done())
		go operatorInformerFactory.Start(ctx.Done())

		go secretInformer.Run(ctx.Done())

		klog.Info("starting klusterlet")
		klusterlet.StartKlusterlet(
			ctx,
			controlplaneCRDClient,
			controlplaneOperatorClient.OperatorV1().Klusterlets(),
			kubeClient,
			workClient,
			kubeInformerFactory,
			operatorInformerFactory.Operator().V1().Klusterlets(),
		)

		if err := mgr.Start(ctx); err != nil {
			klog.Error(err)
		}

		<-ctx.Done()
	}()

	return nil
}
