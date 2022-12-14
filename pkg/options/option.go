/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package options

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/pflag"
	apiextensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/util/keyutil"
	"k8s.io/klog"
	aggregatorscheme "k8s.io/kube-aggregator/pkg/apiserver/scheme"
	"k8s.io/kubernetes/pkg/api/legacyscheme"
	"k8s.io/kubernetes/pkg/kubeapiserver"
	kubeauthenticator "k8s.io/kubernetes/pkg/kubeapiserver/authenticator"
	"k8s.io/kubernetes/pkg/serviceaccount"
	"open-cluster-management.io/multicluster-controlplane/pkg/etcd"
)

// Options runs a kubernetes api server.
type Options struct {
	Generic        *GenericOptions
	Authentication *BuiltInAuthenticationOptions
	Authorization  *BuiltInAuthorizationOptions
	Net            *NetOptions
	EmbeddedEtcd   *EmbeddedEtcdOptions
}

// CompletedOptions is a private wrapper that enforces a call of Complete() before Run can be invoked.
type CompletedOptions struct {
	*Options
}

// NewServerRunOptions creates a new ServerRunOptions object with default parameters
func NewOptions() *Options {
	return &Options{
		Generic:        NewGenericOptions(),
		Authentication: NewBuiltInAuthenticationOptions().WithAll(),
		Authorization:  NewBuiltInAuthorizationOptions(),
		Net:            NewNetOptions(),
		EmbeddedEtcd:   NewEmbeddedEtcdOptions(),
	}
}

func (options *Options) AddFlags(fs *pflag.FlagSet) {
	options.Generic.addFlags(fs)
	options.Authentication.addFlags(fs)
	options.Authorization.addFlags(fs)
	options.EmbeddedEtcd.addFlags(fs)
	options.Net.addFlags(fs)
}

// Complete set default Options.
// Should be called after kube-apiserver flags parsed.
func (options *Options) CompletedAndValidateOptions(stopCh <-chan struct{}) (CompletedOptions, error) {
	completedOptions := CompletedOptions{}
	// set defaults
	if err := options.Generic.GenericServerRunOptions.DefaultAdvertiseAddress(options.Generic.SecureServing.SecureServingOptions); err != nil {
		return completedOptions, err
	}

	if err := options.Net.complete(); err != nil {
		return completedOptions, err
	}

	// SecureServing signed certs
	if err := options.Generic.SecureServing.MaybeDefaultWithSelfSignedCerts(
		options.Generic.GenericServerRunOptions.AdvertiseAddress.String(),
		[]string{"kubernetes.default.svc", "kubernetes.default", "kubernetes"},
		[]net.IP{options.Net.APIServerServiceIP}); err != nil {
		return completedOptions, fmt.Errorf("error creating self-signed certificates: %v", err)
	}

	// externalHost
	if len(options.Generic.GenericServerRunOptions.ExternalHost) == 0 {
		if len(options.Generic.GenericServerRunOptions.AdvertiseAddress) > 0 {
			options.Generic.GenericServerRunOptions.ExternalHost = options.Generic.GenericServerRunOptions.AdvertiseAddress.String()
		} else {
			if hostname, err := os.Hostname(); err == nil {
				options.Generic.GenericServerRunOptions.ExternalHost = hostname
			} else {
				return completedOptions, fmt.Errorf("error finding host name: %v", err)
			}
		}
		klog.Infof("external host was not specified, using %v", options.Generic.GenericServerRunOptions.ExternalHost)
	}

	options.Authentication.ApplyAuthorization(options.Authorization)

	// Use (ServiceAccountSigningKeyFile != "") as a proxy to the user enabling
	// TokenRequest functionality. This defaulting was convenient, but messed up
	// a lot of people when they rotated their serving cert with no idea it was
	// connected to their service account keys. We are taking this opportunity to
	// remove this problematic defaulting.
	if options.Generic.ServiceAccountSigningKeyFile == "" {
		// Default to the private server key for service account token signing
		if len(options.Authentication.ServiceAccounts.KeyFiles) == 0 && options.Generic.SecureServing.ServerCert.CertKey.KeyFile != "" {
			if kubeauthenticator.IsValidServiceAccountKeyFile(options.Generic.SecureServing.ServerCert.CertKey.KeyFile) {
				options.Authentication.ServiceAccounts.KeyFiles = []string{
					options.Generic.SecureServing.ServerCert.CertKey.KeyFile,
				}
			} else {
				klog.Warning("No TLS key provided, service account token authentication disabled")
			}
		}
	}

	// Generic
	if options.Generic.ServiceAccountSigningKeyFile != "" && len(options.Authentication.ServiceAccounts.Issuers) != 0 && options.Authentication.ServiceAccounts.Issuers[0] != "" {

		if options.Authentication.ServiceAccounts.MaxExpiration != 0 {
			lowBound := time.Hour
			upBound := time.Duration(1<<32) * time.Second
			if options.Authentication.ServiceAccounts.MaxExpiration < lowBound ||
				options.Authentication.ServiceAccounts.MaxExpiration > upBound {
				return completedOptions, fmt.Errorf("the service-account-max-token-expiration must be between 1 hour and 2^32 seconds")
			}
			if options.Authentication.ServiceAccounts.ExtendExpiration {
				if options.Authentication.ServiceAccounts.MaxExpiration < serviceaccount.WarnOnlyBoundTokenExpirationSeconds*time.Second {
					klog.Warningf("service-account-extend-token-expiration is true, in order to correctly trigger safe transition logic, service-account-max-token-expiration must be set longer than %d seconds (currently %s)", serviceaccount.WarnOnlyBoundTokenExpirationSeconds, options.Authentication.ServiceAccounts.MaxExpiration)
				}
				if options.Authentication.ServiceAccounts.MaxExpiration < serviceaccount.ExpirationExtensionSeconds*time.Second {
					klog.Warningf("service-account-extend-token-expiration is true, enabling tokens valid up to %d seconds, which is longer than service-account-max-token-expiration set to %s seconds", serviceaccount.ExpirationExtensionSeconds, options.Authentication.ServiceAccounts.MaxExpiration)
				}
			}
		}
		options.Generic.ServiceAccountTokenMaxExpiration = options.Authentication.ServiceAccounts.MaxExpiration

		sk, err := keyutil.PrivateKeyFromFile(options.Generic.ServiceAccountSigningKeyFile)
		if err != nil {
			return completedOptions, fmt.Errorf("failed to parse service-account-issuer-key-file: %v", err)
		}
		options.Generic.ServiceAccountIssuer, err = serviceaccount.JWTTokenGenerator(options.Authentication.ServiceAccounts.Issuers[0], sk)
		if err != nil {
			return completedOptions, fmt.Errorf("failed to build token generator: %v", err)
		}
	}

	// Etcd
	if options.Generic.Etcd.EnableWatchCache {
		sizes := kubeapiserver.DefaultWatchCacheSizes()
		// Ensure that overrides parse correctly.
		userSpecified, err := parseWatchCacheSizes(options.Generic.Etcd.WatchCacheSizes)
		if err != nil {
			return completedOptions, err
		}
		for resource, size := range userSpecified {
			sizes[resource] = size
		}
		options.Generic.Etcd.WatchCacheSizes, err = writeWatchCacheSizes(sizes)
		if err != nil {
			return completedOptions, err
		}
	}

	// completed etcd StorageConfig with embedded server
	if options.EmbeddedEtcd.Enabled {
		klog.Infof("The Embedded etcd directory: %s", options.EmbeddedEtcd.Directory)
		embeddedEtcdServer := &etcd.Server{
			Dir: options.EmbeddedEtcd.Directory,
		}
		shutdownCtx, cancel := context.WithCancel(context.TODO())
		go func() {
			defer cancel()
			<-stopCh
			klog.Infof("Received SIGTERM or SIGINT signal, shutting down controller.")
		}()
		embeddedClientInfo, err := embeddedEtcdServer.Run(shutdownCtx, options.EmbeddedEtcd.PeerPort, options.EmbeddedEtcd.ClientPort, options.EmbeddedEtcd.WalSizeBytes)
		if err != nil {
			return completedOptions, err
		}
		options.Generic.Etcd.StorageConfig.Transport.ServerList = embeddedClientInfo.Endpoints
		options.Generic.Etcd.StorageConfig.Transport.KeyFile = embeddedClientInfo.KeyFile
		options.Generic.Etcd.StorageConfig.Transport.CertFile = embeddedClientInfo.CertFile
		options.Generic.Etcd.StorageConfig.Transport.TrustedCAFile = embeddedClientInfo.TrustedCAFile
	}

	for key, value := range options.Generic.APIEnablement.RuntimeConfig {
		if key == "v1" || strings.HasPrefix(key, "v1/") || key == "api/v1" || strings.HasPrefix(key, "api/v1/") {
			delete(options.Generic.APIEnablement.RuntimeConfig, key)
			options.Generic.APIEnablement.RuntimeConfig["/v1"] = value
		}
		if key == "api/legacy" {
			delete(options.Generic.APIEnablement.RuntimeConfig, key)
		}
	}

	// validate
	errs := []error{}
	errs = append(errs, options.Generic.Etcd.Validate()...)
	errs = append(errs, options.Generic.SecureServing.Validate()...)
	errs = append(errs, options.Authentication.Validate()...)
	errs = append(errs, options.Authorization.Validate()...)
	errs = append(errs, options.Generic.Audit.Validate()...)
	errs = append(errs, options.Generic.Admission.Validate()...)
	errs = append(errs, options.Generic.APIEnablement.Validate(legacyscheme.Scheme, apiextensionsapiserver.Scheme, aggregatorscheme.Scheme)...)
	errs = append(errs, options.Generic.Metrics.Validate()...)
	errs = append(errs, options.EmbeddedEtcd.Validate()...)

	// validate TokenRequest
	enableAttempted := options.Generic.ServiceAccountSigningKeyFile != "" ||
		(len(options.Authentication.ServiceAccounts.Issuers) != 0 && options.Authentication.ServiceAccounts.Issuers[0] != "") || len(options.Authentication.APIAudiences) != 0
	enableSucceeded := options.Generic.ServiceAccountIssuer != nil
	if !enableAttempted {
		errs = append(errs, errors.New("--service-account-signing-key-file and --service-account-issuer are required flags"))
	}
	if enableAttempted && !enableSucceeded {
		errs = append(errs, errors.New("--service-account-signing-key-file, --service-account-issuer, and --api-audiences should be specified together"))
	}

	completedOptions.Options = options
	return completedOptions, utilerrors.NewAggregate(errs)
}

// ParseWatchCacheSizes turns a list of cache size values into a map of group resources
// to requested sizes.
func parseWatchCacheSizes(cacheSizes []string) (map[schema.GroupResource]int, error) {
	watchCacheSizes := make(map[schema.GroupResource]int)
	for _, c := range cacheSizes {
		tokens := strings.Split(c, "#")
		if len(tokens) != 2 {
			return nil, fmt.Errorf("invalid value of watch cache size: %s", c)
		}

		size, err := strconv.Atoi(tokens[1])
		if err != nil {
			return nil, fmt.Errorf("invalid size of watch cache size: %s", c)
		}
		if size < 0 {
			return nil, fmt.Errorf("watch cache size cannot be negative: %s", c)
		}
		watchCacheSizes[schema.ParseGroupResource(tokens[0])] = size
	}
	return watchCacheSizes, nil
}

// WriteWatchCacheSizes turns a map of cache size values into a list of string specifications.
func writeWatchCacheSizes(watchCacheSizes map[schema.GroupResource]int) ([]string, error) {
	var cacheSizes []string

	for resource, size := range watchCacheSizes {
		if size < 0 {
			return nil, fmt.Errorf("watch cache size cannot be negative for resource %s", resource)
		}
		cacheSizes = append(cacheSizes, fmt.Sprintf("%s#%d", resource.String(), size))
	}
	return cacheSizes, nil
}
