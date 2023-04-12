// Copyright Contributors to the Open Cluster Management project

package agent

import (
	"context"
	"embed"
	"strings"

	gktemplatesv1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1"
	gktemplatesv1beta1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1beta1"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/spf13/cobra"
	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"
	v1 "k8s.io/api/core/v1"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog"
	"open-cluster-management.io/addon-framework/pkg/assets"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/governance-policy-propagator/controllers/common"
	"open-cluster-management.io/multicluster-controlplane/pkg/agent"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-controlplane/pkg/config"
	"github.com/stolostron/multicluster-controlplane/pkg/constants"
	"github.com/stolostron/multicluster-controlplane/pkg/controllers/managedclusteraddons"
)

var (
	scheme        = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(scheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

//go:embed crds
var crds embed.FS

var crdStaticFiles = []string{
	"crds/policy.open-cluster-management.io_configurationpolicies_crd.yaml",
	"crds/policy.open-cluster-management.io_policies_crd.yaml",
}

func NewAgent() *cobra.Command {
	agentOptions := newAgentOptions()

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start a Multicluster Controlplane Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			shutdownCtx, cancel := context.WithCancel(context.TODO())

			shutdownHandler := genericapiserver.SetupSignalHandler()
			go func() {
				defer cancel()
				<-shutdownHandler
				klog.Infof("Received SIGTERM or SIGINT signal, shutting down agent.")
			}()

			ctx, terminate := context.WithCancel(shutdownCtx)
			defer terminate()

			opts := zap.Options{
				// enable development mode for more human-readable output, extra stack traces and logging information, etc
				// disable this in final release
				Development: true,
			}
			ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

			kubeConfig, err := clientcmd.BuildConfigFromFlags("", agentOptions.RegistrationAgent.SpokeKubeconfig)
			if err != nil {
				return err
			}
			apiExtensionsClient, err := apiextensionsclient.NewForConfig(kubeConfig)
			if err != nil {
				return err
			}

			if err := ensureCRDs(ctx, apiExtensionsClient); err != nil {
				return err
			}

			hubManager, err := createHubManager(agentOptions)
			if err != nil {
				return err
			}
			manager, err := createManager(agentOptions)
			if err != nil {
				return err
			}

			if utilfeature.DefaultMutableFeatureGate.Enabled(constants.ManagedClusterInfo) {
				// start managed cluster info controller
				go func() {
					klog.Info("starting managed cluster info addon agent")
					err := managedclusteraddons.SetupManagedClusterInfoWithManager(ctx, hubManager, agentOptions)
					if err != nil {
						klog.Fatalf("failed to setup managedclusterinfo addon, %v", err)
					}
				}()
			}

			if utilfeature.DefaultMutableFeatureGate.Enabled(constants.ConfigurationPolicy) {
				go func() {
					klog.Info("starting configuration policy addon agent")
					err = managedclusteraddons.SetupPolicyAddonWithManager(ctx, hubManager, manager, agentOptions)
					if err != nil {
						klog.Fatalf("failed to setup policy addon, %v", err)
					}
				}()
			}

			// start hub runtime manager
			go func() {
				klog.Info("Starting the embedded hub controller-runtime manager in controlplane agent")
				if err := hubManager.Start(ctx); err != nil {
					klog.Fatalf("failed to start embedded hub controller-runtime manager, %v", err)
				}
			}()

			go func() {
				klog.Info("Starting the embedded controller-runtime manager in controlplane agent")
				if err := manager.Start(ctx); err != nil {
					klog.Fatalf("failed to start embedded controller-runtime manager, %v", err)
				}
			}()

			klog.Info("Starting the controlplane agent")
			if err := agentOptions.RunAgent(ctx); err != nil {
				return err
			}
			<-ctx.Done()
			return nil
		},
	}

	flags := cmd.Flags()
	agentOptions.AddFlags(flags)
	return cmd
}

func newAgentOptions() *config.AgentOptions {
	return &config.AgentOptions{
		AgentOptions: *agent.NewAgentOptions(),
		//TODO: pass them via parameters
		DecryptionConcurrency: 5,
		EvaluationConcurrency: 2,
		EnableMetrics:         true,
		Frequency:             10,
	}
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	utilruntime.Must(clusterinfov1beta1.AddToScheme(scheme))
	utilruntime.Must(policyv1.AddToScheme(scheme))
	utilruntime.Must(configpolicyv1.AddToScheme(scheme))
	utilruntime.Must(extensionsv1.AddToScheme(scheme))
	utilruntime.Must(gktemplatesv1.AddToScheme(scheme))
	utilruntime.Must(gktemplatesv1beta1.AddToScheme(scheme))
}

func createHubManager(agentConfig *config.AgentOptions) (manager.Manager, error) {
	// TODO: should use o.registrationAgent.HubKubeconfigDir + "/kubeconfig"
	hubConfig, err := clientcmd.BuildConfigFromFlags("", agentConfig.RegistrationAgent.BootstrapKubeconfig)
	if err != nil {
		return nil, err
	}

	mgr, err := ctrl.NewManager(hubConfig, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      ":8383",
		Namespace:               agentConfig.RegistrationAgent.ClusterName,
		LeaderElection:          true,
		LeaderElectionID:        "multicluster-controlplane-agent-hub-controller-manager",
		LeaderElectionNamespace: agentConfig.RegistrationAgent.ClusterName,
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

func createManager(agentConfig *config.AgentOptions) (manager.Manager, error) {
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", agentConfig.RegistrationAgent.SpokeKubeconfig)
	if err != nil {
		return nil, err
	}

	crdLabelSelector := labels.SelectorFromSet(map[string]string{common.APIGroup + "/policy-type": "template"})
	mgr, err := ctrl.NewManager(kubeConfig, ctrl.Options{
		Scheme:                  scheme,
		MetricsBindAddress:      ":8384",
		LeaderElection:          true,
		LeaderElectionID:        "multicluster-controlplane-agent-controller-manager",
		LeaderElectionNamespace: agentConfig.RegistrationAgent.ClusterName,
		NewCache: cache.BuilderWithOptions(
			cache.Options{
				SelectorsByObject: cache.SelectorsByObject{
					&extensionsv1.CustomResourceDefinition{}: {
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

func ensureCRDs(ctx context.Context, client apiextensionsclient.Interface) error {
	for _, crdFileName := range crdStaticFiles {
		template, err := crds.ReadFile(crdFileName)
		if err != nil {
			return err
		}

		objData := assets.MustCreateAssetFromTemplate(crdFileName, template, nil).Data
		obj, _, err := genericCodec.Decode(objData, nil, nil)
		if err != nil {
			return err
		}

		switch required := obj.(type) {
		case *extensionsv1.CustomResourceDefinition:
			if _, _, err := resourceapply.ApplyCustomResourceDefinitionV1(
				ctx,
				client.ApiextensionsV1(),
				//TODO: use agentConfig.eventRecorder
				events.NewInMemoryRecorder("managed-cluster-agents"),
				required,
			); err != nil {
				return err
			}
		}
	}

	return nil
}
