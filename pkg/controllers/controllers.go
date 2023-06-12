package controllers

import (
	"fmt"
	"time"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"
	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorclient "open-cluster-management.io/api/client/operator/clientset/versioned"
	operatorinformer "open-cluster-management.io/api/client/operator/informers/externalversions"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	clusterv1beta1 "open-cluster-management.io/api/cluster/v1beta1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	policyv1beta1 "open-cluster-management.io/governance-policy-propagator/api/v1beta1"
	placementrulev1 "open-cluster-management.io/multicloud-operators-subscription/pkg/apis/apps/placementrule/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/features"
	"open-cluster-management.io/multicluster-controlplane/pkg/util"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/addons"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/klusterlet"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/manifests"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
	"github.com/stolostron/multicluster-controlplane/pkg/helpers"
)

var requiredCRDs = []string{
	"crds/apps.open-cluster-management.io_placementrules.crd.yaml",
	"crds/internal.open-cluster-management.io_managedclusterinfos.crd.yaml",
	"crds/policy.open-cluster-management.io_placementbindings.crd.yaml",
	"crds/operator.open-cluster-management.io_klusterlets.crd.yaml",
	"crds/policy.open-cluster-management.io_policies.crd.yaml",
	"crds/policy.open-cluster-management.io_policyautomations.crd.yaml",
	"crds/policy.open-cluster-management.io_policysets.crd.yaml",
}

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(kubescheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterv1beta1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(policyv1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))
	utilruntime.Must(placementrulev1.AddToScheme(scheme))
}

// InstallControllers installs next-gen controlplane controllers in hub cluster
func InstallControllers(stopCh <-chan struct{}, aggregatorConfig *aggregatorapiserver.Config) error {
	ctx := util.GoContext(stopCh)
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

	if err := helpers.EnsureCRDs(ctx, controlplaneCRDClient, manifests.CRDFiles, requiredCRDs...); err != nil {
		return err
	}

	go func() {
		mgr, err := ctrl.NewManager(loopbackRestConfig, ctrl.Options{
			Scheme:             scheme,
			MetricsBindAddress: "0", //TODO think about the mertics later
			Cache: cache.Options{
				ByObject: map[client.Object]cache.ByObject{
					&policyv1.Policy{}: {
						Transform: func(obj interface{}) (interface{}, error) {
							k8sObj, ok := obj.(client.Object)
							if !ok {
								return nil, fmt.Errorf("invalid type")
							}
							k8sObj.SetManagedFields(nil)
							return k8sObj, nil
						},
					},
				},
			},
		})
		if err != nil {
			klog.Fatalf("unable to start manager %v", err)
		}

		if features.DefaultControlplaneMutableFeatureGate.Enabled(feature.ManagedClusterInfo) {
			klog.Info("starting managed cluster info addon")
			if err := addons.SetupManagedClusterInfoWithManager(ctx, mgr); err != nil {
				klog.Fatalf("failed to setup managedclusterinfo controller %v", err)
			}
		}

		if features.DefaultControlplaneMutableFeatureGate.Enabled(feature.ConfigurationPolicy) {
			klog.Info("starting policy addon")
			if err := addons.SetupPolicyWithManager(
				ctx,
				mgr,
				loopbackRestConfig,
				controlplaneKubeClient,
				controlplaneDynamicClient,
			); err != nil {
				klog.Fatalf("failed to setup policy controller %v", err)
			}
		}

		if restConfig, err := rest.InClusterConfig(); err == nil {
			controlplaneOperatorClient, err := operatorclient.NewForConfig(loopbackRestConfig)
			if err != nil {
				klog.Fatalf("failed to build controlplane operator client %v", err)
			}

			controlplaneClusterClient, err := clusterclient.NewForConfig(loopbackRestConfig)
			if err != nil {
				klog.Fatalf("failed to build controlplane cluster client %v", err)
			}

			controlplaneWorkClient, err := workclient.NewForConfig(loopbackRestConfig)
			if err != nil {
				klog.Fatalf("failed to build controlplane work client %v", err)
			}

			kubeClient, err := kubernetes.NewForConfig(restConfig)
			if err != nil {
				klog.Fatalf("failed to build kube client on the management cluster %v", err)
			}

			workClient, err := workclient.NewForConfig(restConfig)
			if err != nil {
				klog.Fatalf("failed to build work client on the management cluster %v", err)
			}

			kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Minute)
			operatorInformerFactory := operatorinformer.NewSharedInformerFactory(controlplaneOperatorClient, 10*time.Minute)

			klog.Info("starting klusterlet")
			klusterlet := klusterlet.NewKlusterlet(
				controlplaneKubeClient,
				controlplaneDynamicClient,
				controlplaneClusterClient,
				controlplaneWorkClient,
				controlplaneCRDClient,
				controlplaneOperatorClient.OperatorV1().Klusterlets(),
				kubeClient,
				workClient.WorkV1().AppliedManifestWorks(),
				kubeInformerFactory,
				operatorInformerFactory.Operator().V1().Klusterlets(),
			)

			go kubeInformerFactory.Start(ctx.Done())
			go operatorInformerFactory.Start(ctx.Done())

			klusterlet.Start(ctx)
		}

		if err := mgr.Start(ctx); err != nil {
			klog.Fatalf("failed to start controller manager, %v", err)
		}

		<-ctx.Done()
	}()

	return nil
}
