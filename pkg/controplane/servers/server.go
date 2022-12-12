package servers

import (
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	genericapifilters "k8s.io/apiserver/pkg/endpoints/filters"
	genericapiserver "k8s.io/apiserver/pkg/server"
	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/apiserver/pkg/util/notfoundhandler"
	"k8s.io/apiserver/pkg/util/webhook"
	aggregatorapiserver "k8s.io/kube-aggregator/pkg/apiserver"
	ocmfeature "open-cluster-management.io/api/feature"

	"github.com/stolostron/multicluster-controlplane/pkg/controplane/options"
)

func init() {
	utilruntime.Must(utilfeature.DefaultMutableFeatureGate.Add(ocmfeature.DefaultHubWorkFeatureGates))
	utilruntime.Must(utilfeature.DefaultMutableFeatureGate.Add(ocmfeature.DefaultHubRegistrationFeatureGates))
	utilruntime.Must(utilfeature.DefaultMutableFeatureGate.Add(ocmfeature.DefaultSpokeRegistrationFeatureGates))
}

// CreateServerChain creates the apiservers connected via delegation.
func CreateServerChain(completedOptions options.CompletedOptions) (*aggregatorapiserver.APIAggregator, error) {
	kubeAPIServerConfig, serviceResolver, pluginInitializer, err := createKubeAPIServerConfig(completedOptions)
	if err != nil {
		return nil, err
	}

	// If additional API servers are added, they should be gated.
	apiExtensionsConfig, err := createAPIExtensionsConfig(
		*kubeAPIServerConfig.GenericConfig,
		kubeAPIServerConfig.ExtraConfig.VersionedInformers,
		pluginInitializer, completedOptions.Generic,
		1, serviceResolver,
		webhook.NewDefaultAuthenticationInfoResolverWrapper(kubeAPIServerConfig.ExtraConfig.ProxyTransport, kubeAPIServerConfig.GenericConfig.EgressSelector, kubeAPIServerConfig.GenericConfig.LoopbackClientConfig, kubeAPIServerConfig.GenericConfig.TracerProvider))
	if err != nil {
		return nil, err
	}

	notFoundHandler := notfoundhandler.New(kubeAPIServerConfig.GenericConfig.Serializer, genericapifilters.NoMuxAndDiscoveryIncompleteKey)
	apiExtensionsServer, err := createAPIExtensionsServer(apiExtensionsConfig,
		genericapiserver.NewEmptyDelegateWithCustomHandler(notFoundHandler))
	if err != nil {
		return nil, err
	}

	kubeAPIServer, err := CreateKubeAPIServer(kubeAPIServerConfig, apiExtensionsServer.GenericAPIServer)
	if err != nil {
		return nil, err
	}

	// aggregator comes last in the chain
	aggregatorConfig, err := createAggregatorConfig(*kubeAPIServerConfig.GenericConfig, completedOptions.Generic, kubeAPIServerConfig.ExtraConfig.VersionedInformers, serviceResolver, kubeAPIServerConfig.ExtraConfig.ProxyTransport, pluginInitializer)
	if err != nil {
		return nil, err
	}
	aggregatorServer, err := createAggregatorServer(
		aggregatorConfig, kubeAPIServer.GenericAPIServer, apiExtensionsServer.Informers,
		completedOptions.Authentication.ClientCert.ClientCA,
		completedOptions.Generic.ClientKeyFile,
	)
	if err != nil {
		return nil, err
	}

	return aggregatorServer, nil
}
