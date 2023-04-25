package agent

import (
	"context"
	"strings"

	gktemplatesv1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1"
	gktemplatesv1beta1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1beta1"
	openshiftclientset "github.com/openshift/client-go/config/clientset/versioned"
	openshiftoauthclientset "github.com/openshift/client-go/oauth/clientset/versioned"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	clusterclientset "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	operatorapiv1 "open-cluster-management.io/api/operator/v1"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	"open-cluster-management.io/config-policy-controller/controllers"
	configcommon "open-cluster-management.io/config-policy-controller/pkg/common"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/governance-policy-propagator/controllers/common"
	msav1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
	"open-cluster-management.io/multicluster-controlplane/pkg/features"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-controlplane/pkg/agent/addons"
	"github.com/stolostron/multicluster-controlplane/pkg/agent/addons/manifests"
	"github.com/stolostron/multicluster-controlplane/pkg/feature"
	"github.com/stolostron/multicluster-controlplane/pkg/helpers"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(kubescheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(configpolicyv1.AddToScheme(scheme))
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme))
	utilruntime.Must(msav1alpha1.AddToScheme(scheme))
	utilruntime.Must(gktemplatesv1.AddToScheme(scheme))
	utilruntime.Must(gktemplatesv1beta1.AddToScheme(scheme))
}

var requiredCRDFiles = []string{
	"crds/clusters.open-cluster-management.io_clusterclaims.crd.yaml",
	"crds/policy.open-cluster-management.io_configurationpolicies.crd.yaml",
	"crds/policy.open-cluster-management.io_policies.crd.yaml",
	"crds/work.open-cluster-management.io_appliedmanifestworks.crd.yaml",
}

type AgentOptions struct {
	*agent.AgentOptions
	*addons.PolicyAgentConfig
	DeployMode operatorapiv1.InstallMode
}

func NewAgentOptions() *AgentOptions {
	return &AgentOptions{
		AgentOptions: agent.NewAgentOptions(),
		PolicyAgentConfig: &addons.PolicyAgentConfig{
			// TODO pass them via parameters
			DecryptionConcurrency: 5,
			EvaluationConcurrency: 2,
			EnableMetrics:         true,
			Frequency:             10,
		},
	}
}

func (a *AgentOptions) RunAddOns(ctx context.Context) error {
	// TODO should use o.registrationAgent.HubKubeconfigDir + "/kubeconfig"
	hubKubeConfig, err := clientcmd.BuildConfigFromFlags("", a.RegistrationAgent.BootstrapKubeconfig)
	if err != nil {
		return err
	}

	spokeKubeConfig, err := clientcmd.BuildConfigFromFlags("", a.RegistrationAgent.SpokeKubeconfig)
	if err != nil {
		return err
	}

	kubeClient, err := kubernetes.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	dynamicClient, err := dynamic.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	crdClient, err := apiextensionsclient.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	if err := helpers.EnsureCRDs(ctx, crdClient, manifests.AgentCRDFiles, requiredCRDFiles...); err != nil {
		return err
	}

	opts := &zap.Options{
		// enable development mode for more human-readable output, extra stack traces and logging information, etc
		// disable this in final release
		Development: true,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(opts)))

	hubManager, err := a.newHubManager(hubKubeConfig, scheme)
	if err != nil {
		return err
	}

	spokeManager, err := a.newSpokeManager(spokeKubeConfig, scheme)
	if err != nil {
		return err
	}

	var hostingManager manager.Manager
	if a.DeployMode == operatorapiv1.InstallModeHosted {
		hostingManager, err = a.newHostingManager(scheme)
		if err != nil {
			return err
		}
	}

	clusterName := a.RegistrationAgent.ClusterName
	startCtrlMgr := false

	if features.DefaultAgentMutableFeatureGate.Enabled(feature.ManagedClusterInfo) {
		// start managed cluster info controller
		klog.Info("starting managed cluster info addon agent")
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

		restMapper, err := apiutil.NewDynamicRESTMapper(spokeKubeConfig, apiutil.WithLazyDiscovery)
		if err != nil {
			return err
		}

		if err := addons.StartManagedClusterInfoAgent(
			ctx,
			clusterName,
			hubManager,
			kubeClient,
			dynamicClient,
			clusterClient,
			ocpClient,
			ocpOauthClient,
			restMapper,
		); err != nil {
			return err
		}

		startCtrlMgr = true
	}

	if features.DefaultAgentMutableFeatureGate.Enabled(feature.ConfigurationPolicy) {
		klog.Info("starting configuration policy addon agent")
		if err := addons.StartPolicyAgent(
			ctx,
			clusterName,
			spokeKubeConfig,
			hubManager,
			spokeManager,
			hostingManager,
			kubeClient,
			dynamicClient,
			a.PolicyAgentConfig,
		); err != nil {
			klog.Fatalf("failed to setup policy addon, %v", err)
		}

		startCtrlMgr = true
	}

	if features.DefaultAgentMutableFeatureGate.Enabled(feature.ManagedServiceAccount) {
		klog.Info("starting managed serviceaccount addon agent")
		if err := addons.StartManagedServiceAccountAgent(ctx, hubManager, clusterName); err != nil {
			klog.Fatalf("failed to setup managed serviceaccount addon, %v", err)
		}

		startCtrlMgr = true
	}

	if !startCtrlMgr {
		klog.Info("no addons are enabled")
		return nil
	}

	// start hub runtime manager
	go func() {
		klog.Info("starting the embedded hub controller-runtime manager in controlplane agent")
		if err := hubManager.Start(ctx); err != nil {
			klog.Fatalf("failed to start embedded hub controller-runtime manager, %v", err)
		}
	}()

	go func() {
		klog.Info("starting the embedded controller-runtime manager in controlplane agent")
		if err := spokeManager.Start(ctx); err != nil {
			klog.Fatalf("failed to start embedded controller-runtime manager, %v", err)
		}
	}()

	return nil
}

func (a *AgentOptions) newHubManager(config *rest.Config, scheme *runtime.Scheme) (manager.Manager, error) {
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:             scheme,
		Namespace:          a.RegistrationAgent.ClusterName,
		MetricsBindAddress: "0", //TODO think about the mertics later
		NewCache: cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: cache.SelectorsByObject{
					&v1.Secret{}: {
						Field: fields.SelectorFromSet(fields.Set{"metadata.name": secretsync.SecretName}),
					},
				},
			},
		),
		// Override the EventBroadcaster so that the spam filter will not ignore events for the policy but with
		// different messages if a large amount of events for that policy are sent in a short time.
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(
			record.CorrelatorOptions{
				// This essentially disables event aggregation of the same events but with different messages.
				MaxIntervalInSeconds: 1,
				// This is the default spam key function except it adds the reason and message as well.
				// https://github.com/kubernetes/client-go/blob/v0.23.3/tools/record/events_cache.go#L70-L82
				SpamKeyFunc: func(event *v1.Event) string {
					return strings.Join(
						[]string{
							event.Source.Component,
							event.Source.Host,
							event.InvolvedObject.Kind,
							event.InvolvedObject.Namespace,
							event.InvolvedObject.Name,
							string(event.InvolvedObject.UID),
							event.InvolvedObject.APIVersion,
							event.Reason,
							event.Message,
						},
						"",
					)
				},
			},
		),
	})
	if err != nil {
		return nil, err
	}

	return mgr, nil
}

func (a *AgentOptions) newSpokeManager(config *rest.Config, scheme *runtime.Scheme) (manager.Manager, error) {
	crdLabelSelector := labels.SelectorFromSet(map[string]string{common.APIGroup + "/policy-type": "template"})
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0", //TODO think about the mertics later
		NewCache: cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: cache.SelectorsByObject{
					&apiextensionsv1.CustomResourceDefinition{}: {
						Label: crdLabelSelector,
					},
				},
			},
		),
		// Override the EventBroadcaster so that the spam filter will not ignore events for the policy but with
		// different messages if a large amount of events for that policy are sent in a short time.
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(
			record.CorrelatorOptions{
				// This essentially disables event aggregation of the same events but with different messages.
				MaxIntervalInSeconds: 1,
				// This is the default spam key function except it adds the reason and message as well.
				// https://github.com/kubernetes/client-go/blob/v0.23.3/tools/record/events_cache.go#L70-L82
				SpamKeyFunc: func(event *v1.Event) string {
					return strings.Join(
						[]string{
							event.Source.Component,
							event.Source.Host,
							event.InvolvedObject.Kind,
							event.InvolvedObject.Namespace,
							event.InvolvedObject.Name,
							string(event.InvolvedObject.UID),
							event.InvolvedObject.APIVersion,
							event.Reason,
							event.Message,
						},
						"",
					)
				},
			},
		),
	})
	if err != nil {
		return nil, err
	}

	return mgr, nil
}

func (a *AgentOptions) newHostingManager(scheme *runtime.Scheme) (manager.Manager, error) {

	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	cacheSelectors := cache.SelectorsByObject{
		&apiextensionsv1.CustomResourceDefinition{}: {
			Field: fields.SelectorFromSet(fields.Set{"metadata.name": controllers.CRDName}),
		},
	}

	ctrlKey, err := configcommon.GetOperatorNamespacedName()
	if err != nil {
		return nil, err
	}

	cacheSelectors[&appsv1.Deployment{}] = cache.ObjectSelector{
		Field: fields.SelectorFromSet(fields.Set{
			"metadata.namespace": ctrlKey.Namespace,
			"metadata.name":      ctrlKey.Name,
		}),
	}

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0", //TODO think about the mertics later
		NewCache: cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: cacheSelectors,
			},
		),
		ClientDisableCacheFor: []client.Object{&corev1.Secret{}},
		// Override the EventBroadcaster so that the spam filter will not ignore events for the policy but with
		// different messages if a large amount of events for that policy are sent in a short time.
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(
			record.CorrelatorOptions{
				// This essentially disables event aggregation of the same events but with different messages.
				MaxIntervalInSeconds: 1,
				// This is the default spam key function except it adds the reason and message as well.
				// https://github.com/kubernetes/client-go/blob/v0.23.3/tools/record/events_cache.go#L70-L82
				SpamKeyFunc: func(event *v1.Event) string {
					return strings.Join(
						[]string{
							event.Source.Component,
							event.Source.Host,
							event.InvolvedObject.Kind,
							event.InvolvedObject.Namespace,
							event.InvolvedObject.Name,
							string(event.InvolvedObject.UID),
							event.InvolvedObject.APIVersion,
							event.Reason,
							event.Message,
						},
						"",
					)
				},
			},
		),
	})
	if err != nil {
		return nil, err
	}

	return mgr, nil
}
