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
	saclient "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
)

const (
	timeout        = 60 * time.Second
	addonTimeout   = 120 * time.Second
	agentNamespace = "multicluster-controlplane-agent"
)

var ctx = context.TODO()

var (
	controlPlaneNamespace    string
	managedClusterName       string
	hostedManagedClusterName string

	// management clients
	managementKubeClient    kubernetes.Interface
	managementDynamicClient dynamic.Interface

	// controlplane clients
	kubeClient       kubernetes.Interface
	dynamicClient    dynamic.Interface
	clusterClient    clusterclient.Interface
	workClient       workclient.Interface
	klusterletClient operatorv1client.KlusterletInterface
	saClient         saclient.Interface

	// default spoke clients
	spokeKubeClient    kubernetes.Interface
	spokeClusterClient clusterclient.Interface

	// hosted spoke clients
	hostedSpokeKubeClient    kubernetes.Interface
	hostedSpokeClusterClient clusterclient.Interface
	hostedSpokeWorkClient    workclient.Interface
	hostedCRDsClient         apiextensionsclient.Interface
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	err := func() error {
		controlPlaneNamespace = os.Getenv("CONTROLPLANE_NAMESPACE")
		managedClusterName = os.Getenv("MANAGED_CLUSTER")
		hostedManagedClusterName = os.Getenv("HOSTED_MANAGED_CLUSTER")

		managementConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("MANAGEMENT_KUBECONFIG"))
		if err != nil {
			return err
		}

		managementKubeClient, err = kubernetes.NewForConfig(managementConfig)
		if err != nil {
			return err
		}

		managementDynamicClient, err = dynamic.NewForConfig(managementConfig)
		if err != nil {
			return err
		}

		controlplaneConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("CONTROLPLANE_KUBECONFIG"))
		if err != nil {
			return err
		}

		kubeClient, err = kubernetes.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		dynamicClient, err = dynamic.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		clusterClient, err = clusterclient.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		workClient, err = workclient.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		operatorClient, err := operatorv1client.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}
		klusterletClient = operatorClient.Klusterlets()

		saClient, err = saclient.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		spokeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("MANAGED_CLUSTER_KUBECONFIG"))
		if err != nil {
			return err
		}

		spokeKubeClient, err = kubernetes.NewForConfig(spokeConfig)
		if err != nil {
			return err
		}

		spokeClusterClient, err = clusterclient.NewForConfig(spokeConfig)
		if err != nil {
			return err
		}

		hostedSpokeConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("HOSTED_MANAGED_CLUSTER_KUBECONFIG"))
		if err != nil {
			return err
		}

		hostedSpokeKubeClient, err = kubernetes.NewForConfig(hostedSpokeConfig)
		if err != nil {
			return err
		}

		hostedSpokeClusterClient, err = clusterclient.NewForConfig(hostedSpokeConfig)
		if err != nil {
			return err
		}

		hostedSpokeWorkClient, err = workclient.NewForConfig(hostedSpokeConfig)
		if err != nil {
			return err
		}

		hostedCRDsClient, err = apiextensionsclient.NewForConfig(hostedSpokeConfig)
		if err != nil {
			return err
		}

		return nil
	}()

	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})
