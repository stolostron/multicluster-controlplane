package agent

import (
	"context"
	"fmt"
	"strings"

	gktemplatesv1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1"
	gktemplatesv1beta1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1beta1"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	"open-cluster-management.io/config-policy-controller/controllers"
	configcommon "open-cluster-management.io/config-policy-controller/pkg/common"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
	"open-cluster-management.io/multicluster-controlplane/pkg/features"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	utilruntime.Must(gktemplatesv1.AddToScheme(scheme))
	utilruntime.Must(gktemplatesv1beta1.AddToScheme(scheme))
}

var agentRequiredCRDFiles = []string{
	"crds/clusters.open-cluster-management.io_clusterclaims.crd.yaml",
	"crds/work.open-cluster-management.io_appliedmanifestworks.crd.yaml",
}

var policyRequiredCRDFiles = []string{
	"crds/policy.open-cluster-management.io_configurationpolicies.crd.yaml",
	"crds/policy.open-cluster-management.io_policies.crd.yaml",
}

var correlatorOptions = record.CorrelatorOptions{
	// This essentially disables event aggregation of the same events but with different messages.
	MaxIntervalInSeconds: 1,
	// This is the default spam key function except it adds the reason and message as well.
	// https://github.com/kubernetes/client-go/blob/v0.23.3/tools/record/events_cache.go#L70-L82
	SpamKeyFunc: func(event *corev1.Event) string {
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
}

type AgentOptions struct {
	*agent.AgentOptions
	*addons.PolicyAgentConfig
	hubKubeConfig         *rest.Config
	selfManagementEnabled bool
	clusterName           string
}

func NewAgentOptions() *AgentOptions {
	return &AgentOptions{
		AgentOptions: agent.NewAgentOptions(),
		PolicyAgentConfig: &addons.PolicyAgentConfig{
			// TODO pass them via parameters
			DecryptionConcurrency: 5,
			EvaluationConcurrency: 2,
			Frequency:             10,
		},
	}
}

func (a *AgentOptions) WithHubKubeConfig(hubKubeConfig *rest.Config) *AgentOptions {
	a.hubKubeConfig = hubKubeConfig
	return a
}

func (a *AgentOptions) WithClusterName(clusterName string) *AgentOptions {
	a.clusterName = clusterName
	return a
}

func (a *AgentOptions) WithSelfManagementEnabled(enabled bool) *AgentOptions {
	a.selfManagementEnabled = enabled
	return a
}

func (a *AgentOptions) RunAddOns(ctx context.Context) error {
	var err error

	hubKubeConfig := a.hubKubeConfig
	if hubKubeConfig == nil {
		// TODO should use o.registrationAgent.HubKubeconfigDir + "/kubeconfig"
		hubKubeConfig, err = clientcmd.BuildConfigFromFlags("", a.RegistrationAgent.BootstrapKubeconfig)
		if err != nil {
			return err
		}
	}

	// in hosted mode, the hostingKubeConfig is for the management cluster.
	// in default mode, the hostingKubeConfig is for the managed cluster.
	hostingKubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return err
	}

	// the spokeKubeConfig is always for the managed cluster.
	spokeKubeConfig, err := clientcmd.BuildConfigFromFlags("", a.RegistrationAgent.AgentOptions.SpokeKubeconfigFile)
	if err != nil {
		return err
	}

	hostingCRDClient, err := apiextensionsclient.NewForConfig(hostingKubeConfig)
	if err != nil {
		return err
	}

	spokeCRDClient, err := apiextensionsclient.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	if err := helpers.EnsureCRDs(ctx, scheme, spokeCRDClient, manifests.AgentCRDFiles, agentRequiredCRDFiles...); err != nil {
		return err
	}

	if err := helpers.EnsureCRDs(ctx, scheme, hostingCRDClient, manifests.AgentCRDFiles, policyRequiredCRDFiles...); err != nil {
		return err
	}

	clusterName := a.clusterName
	if len(clusterName) == 0 {
		clusterName = a.RegistrationAgent.AgentOptions.SpokeClusterName
	}

	if features.DefaultAgentMutableFeatureGate.Enabled(feature.ManagedClusterInfo) {
		// start managed cluster info controller
		klog.Info("starting managed cluster info addon agent")

		if err := addons.StartManagedClusterInfoAgent(
			ctx,
			clusterName,
			a.selfManagementEnabled,
			hubKubeConfig,
			spokeKubeConfig,
			a.SpokeRestMapper,
			a.SpokeKubeInformerFactory,
			a.SpokeClusterInformerFactory,
		); err != nil {
			return err
		}
	}

	if features.DefaultAgentMutableFeatureGate.Enabled(feature.ConfigurationPolicy) {
		klog.Info("starting configuration policy addon agent")
		hubManager, err := a.newHubManager(hubKubeConfig, clusterName)
		if err != nil {
			return err
		}

		hostingManager, err := a.newHostingManager(hostingKubeConfig, clusterName)
		if err != nil {
			return err
		}

		if err := addons.StartPolicyAgent(
			ctx,
			scheme,
			clusterName,
			hubKubeConfig,
			hostingKubeConfig,
			spokeKubeConfig,
			hubManager,
			hostingManager,
			a.PolicyAgentConfig,
		); err != nil {
			klog.Fatalf("failed to setup policy addon, %v", err)
		}

		go func() {
			klog.Info("starting the embedded hub controller-runtime manager in controlplane agent")
			if err := hubManager.Start(ctx); err != nil {
				klog.Fatalf("failed to start embedded hub controller-runtime manager, %v", err)
			}
		}()

		go func() {
			klog.Info("starting the embedded hosting controller-runtime manager in controlplane agent")
			if err := hostingManager.Start(ctx); err != nil {
				klog.Fatalf("failed to start embedded hosting controller-runtime manager, %v", err)
			}
		}()
	}

	return nil
}

func (a *AgentOptions) newHubManager(hubKubeConfig *rest.Config, clusterName string) (manager.Manager, error) {
	mgr, err := ctrl.NewManager(hubKubeConfig, ctrl.Options{
		Scheme:             scheme,
		Namespace:          clusterName,
		MetricsBindAddress: "0", //TODO think about the mertics later
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&corev1.Secret{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.name": secretsync.SecretName}),
				},
				&policyv1.Policy{}: {
					Field:     fields.SelectorFromSet(fields.Set{"metadata.namespace": clusterName}),
					Transform: transformer,
				},
				&corev1.Event{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": clusterName}),
				},
			},
			Namespaces: []string{clusterName},
		},
		// Override the EventBroadcaster so that the spam filter will not ignore events for the policy but with
		// different messages if a large amount of events for that policy are sent in a short time.
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(correlatorOptions),
	})
	if err != nil {
		return nil, err
	}

	return mgr, nil
}

func (a *AgentOptions) newHostingManager(hostingKubeConfig *rest.Config, clusterName string) (manager.Manager, error) {
	ctrlKey, err := configcommon.GetOperatorNamespacedName()
	if err != nil {
		return nil, err
	}

	mgr, err := ctrl.NewManager(hostingKubeConfig, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: "0", //TODO think about the mertics later
		Cache: cache.Options{
			ByObject: map[client.Object]cache.ByObject{
				&apiextensionsv1.CustomResourceDefinition{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.name": controllers.CRDName}),
				},
				&appsv1.Deployment{}: {
					Field: fields.SelectorFromSet(fields.Set{
						"metadata.namespace": ctrlKey.Namespace,
						"metadata.name":      ctrlKey.Name,
					}),
				},
				&configpolicyv1.ConfigurationPolicy{}: {
					Field:     fields.SelectorFromSet(fields.Set{"metadata.namespace": clusterName}),
					Transform: transformer,
				},
				&policyv1.Policy{}: {
					Field:     fields.SelectorFromSet(fields.Set{"metadata.namespace": clusterName}),
					Transform: transformer,
				},
				&corev1.Event{}: {
					Field: fields.SelectorFromSet(fields.Set{"metadata.namespace": clusterName}),
				},
			},
			Namespaces: []string{ctrlKey.Namespace, clusterName},
		},
		ClientDisableCacheFor: []client.Object{&corev1.Secret{}},
		// Override the EventBroadcaster so that the spam filter will not ignore events for the policy but with
		// different messages if a large amount of events for that policy are sent in a short time.
		EventBroadcaster: record.NewBroadcasterWithCorrelatorOptions(correlatorOptions),
	})
	if err != nil {
		return nil, err
	}

	return mgr, nil
}

// remove unused fields beforing pushing to cache to optimize memory usage
func transformer(obj interface{}) (interface{}, error) {
	k8sObj, ok := obj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("invalid type")
	}
	k8sObj.SetManagedFields(nil)
	return k8sObj, nil
}
