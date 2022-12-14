package options

import (
	"time"

	"github.com/spf13/pflag"
	genericoptions "k8s.io/apiserver/pkg/server/options"
	"k8s.io/apiserver/pkg/storage/storagebackend"
	"k8s.io/component-base/logs"
	"k8s.io/component-base/metrics"
	"k8s.io/kubernetes/pkg/controlplane/reconcilers"
	kubeletclient "k8s.io/kubernetes/pkg/kubelet/client"
	"k8s.io/kubernetes/pkg/serviceaccount"
	netutils "k8s.io/utils/net"
)

// DefaultEtcdPathPrefix is the default key prefix of etcd for API Server
const DefaultEtcdPathPrefix = "/registry"

type GenericOptions struct {
	GenericServerRunOptions *genericoptions.ServerRunOptions
	Etcd                    *genericoptions.EtcdOptions
	SecureServing           *genericoptions.SecureServingOptionsWithLoopback
	Audit                   *genericoptions.AuditOptions
	Features                *genericoptions.FeatureOptions
	APIEnablement           *genericoptions.APIEnablementOptions
	EgressSelector          *genericoptions.EgressSelectorOptions
	Admission               *genericoptions.AdmissionOptions
	Traces                  *genericoptions.TracingOptions

	Metrics  *metrics.Options
	Logs     *logs.Options
	EventTTL time.Duration

	IdentityLeaseDurationSeconds      int
	IdentityLeaseRenewIntervalSeconds int

	AllowPrivileged         bool
	EnableAggregatorRouting bool

	ServiceAccountSigningKeyFile     string
	ServiceAccountIssuer             serviceaccount.TokenGenerator
	ServiceAccountTokenMaxExpiration time.Duration

	KubeletConfig          kubeletclient.KubeletClientConfig
	EndpointReconcilerType string
	ClientKeyFile          string

	ShowHiddenMetricsForVersion string
	MaxConnectionBytesPerSec    int64
}

func NewGenericOptions() *GenericOptions {
	// ETCD
	etcdOptions := genericoptions.NewEtcdOptions(storagebackend.NewDefaultConfig(DefaultEtcdPathPrefix, nil))
	// Overwrite the default for storage data format.
	etcdOptions.DefaultStorageMediaType = "application/vnd.kubernetes.protobuf"

	// SecureServing
	secureServingOptions := genericoptions.SecureServingOptions{
		BindAddress: netutils.ParseIPSloppy("0.0.0.0"),
		BindPort:    6443,
		Required:    true,
		ServerCert: genericoptions.GeneratableKeyCert{
			PairName:      "apiserver",
			CertDirectory: "/var/run/kubernetes",
		},
	}

	// Admission
	admissionOptions := genericoptions.NewAdmissionOptions()
	// register all admission plugins
	RegisterAllAdmissionPlugins(admissionOptions.Plugins)
	// set RecommendedPluginOrder
	admissionOptions.RecommendedPluginOrder = AllOrderedPlugins
	// set DefaultOffPlugins
	admissionOptions.DefaultOffPlugins = DefaultOffAdmissionPlugins()

	return &GenericOptions{
		GenericServerRunOptions: genericoptions.NewServerRunOptions(),
		Etcd:                    etcdOptions,
		SecureServing:           secureServingOptions.WithLoopback(),
		Audit:                   genericoptions.NewAuditOptions(),
		Features:                genericoptions.NewFeatureOptions(),
		APIEnablement:           genericoptions.NewAPIEnablementOptions(),
		EgressSelector:          genericoptions.NewEgressSelectorOptions(),
		Admission:               admissionOptions,
		Traces:                  genericoptions.NewTracingOptions(),

		Metrics:                           metrics.NewOptions(),
		Logs:                              logs.NewOptions(),
		EventTTL:                          1 * time.Hour,
		IdentityLeaseDurationSeconds:      3600,
		IdentityLeaseRenewIntervalSeconds: 10,

		// this is fake config, just to let server start
		KubeletConfig: kubeletclient.KubeletClientConfig{
			Port:         10250,
			ReadOnlyPort: 10255,
			PreferredAddressTypes: []string{
				"Hostname",
				"InternalDNS",
				"InternalIP",
				"ExternalDNS",
				"ExternalIP",
			},
			HTTPTimeout: time.Duration(5) * time.Second,
		},
		EndpointReconcilerType: string(reconcilers.LeaseEndpointReconcilerType),
	}
}

func (genericOptions *GenericOptions) addFlags(fs *pflag.FlagSet) {
	genericOptions.GenericServerRunOptions.AddUniversalFlags(fs)
	genericOptions.SecureServing.AddFlags(fs)
	genericOptions.Etcd.AddFlags(fs)
	genericOptions.Features.AddFlags(fs)
	genericOptions.Admission.AddFlags(fs)

	fs.StringVar(&genericOptions.ServiceAccountSigningKeyFile, "service-account-signing-key-file", genericOptions.ServiceAccountSigningKeyFile, "Path to the file that contains the current private key of the service account token issuer. The issuer will sign issued ID tokens with this private key.")
	fs.StringVar(&genericOptions.ClientKeyFile, "client-key-file", genericOptions.ClientKeyFile, "client cert key file")
}
