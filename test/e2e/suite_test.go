// Copyright Contributors to the Open Cluster Management project

package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	operatorv1client "open-cluster-management.io/api/client/operator/clientset/versioned/typed/operator/v1"
	workclient "open-cluster-management.io/api/client/work/clientset/versioned"
)

const (
	timeout      = 60 * time.Second
	addonTimeout = 120 * time.Second

	agentNamespace  = "multicluster-controlplane-agent"
	policyName      = "policy-limitrange"
	policyNamespace = "default"
	limitrangeName  = "container-mem-limit-range"
)

type clients struct {
	kubeClient       kubernetes.Interface
	dynamicClient    dynamic.Interface
	crdsClient       apiextensionsclient.Interface
	clusterClient    clusterclient.Interface
	workClient       workclient.Interface
	klusterletClient operatorv1client.KlusterletInterface
}

var ctx = context.TODO()

var (
	managementClusterClients *clients

	selfControlplaneClients *clients
	controlplaneClients     *clients

	spokeClients       *clients
	hostedSpokeClients *clients
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	err := func() error {
		var err error

		// init management cluster clients
		managementClusterClients, err = initClinents("MANAGEMENT_KUBECONFIG")
		if err != nil {
			return err
		}

		// init self management clients
		selfControlplaneClients, err = initClinents("SELF_CONTROLPLANE_KUBECONFIG")
		if err != nil {
			return err
		}

		// init controlplane clients
		controlplaneClients, err = initClinents("CONTROLPLANE_KUBECONFIG")
		if err != nil {
			return err
		}

		// init managed cluster clients
		spokeClients, err = initClinents("MANAGED_CLUSTER_KUBECONFIG")
		if err != nil {
			return err
		}

		// init hosted managed cluster clients
		hostedSpokeClients, err = initClinents("HOSTED_MANAGED_CLUSTER_KUBECONFIG")
		if err != nil {
			return err
		}

		return nil
	}()

	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})

func initClinents(configEnv string) (*clients, error) {
	configPath := os.Getenv(configEnv)
	if len(configPath) == 0 {
		ginkgo.GinkgoWriter.Printf("Ignore the env %q, because it is not set.\n", configEnv)
		return nil, nil
	}

	config, err := clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		return nil, err
	}
	kubeClient, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	dynamicClient, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	crdsClient, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	clusterClient, err := clusterclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	workClient, err := workclient.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	operatorClient, err := operatorv1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &clients{
		kubeClient:       kubeClient,
		dynamicClient:    dynamicClient,
		crdsClient:       crdsClient,
		clusterClient:    clusterClient,
		workClient:       workClient,
		klusterletClient: operatorClient.Klusterlets(),
	}, nil
}
