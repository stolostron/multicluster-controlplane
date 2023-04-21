// Copyright Contributors to the Open Cluster Management project

package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	saclient "open-cluster-management.io/managed-serviceaccount/pkg/generated/clientset/versioned"
)

const (
	timeout        = 60 * time.Second
	addonTimeout   = 120 * time.Second
	agentNamespace = "multicluster-controlplane-agent"
)

var ctx = context.TODO()

var (
	controlPlaneName   string
	managedClusterName string

	// hub clients
	hubKubeClient kubernetes.Interface
	clusterClient clusterclient.Interface
	saClient      saclient.Interface

	// spoke clients
	spokeKubeClient kubernetes.Interface
)

func TestE2E(t *testing.T) {
	gomega.RegisterFailHandler(ginkgo.Fail)
	ginkgo.RunSpecs(t, "E2E Suite")
}

var _ = ginkgo.BeforeSuite(func() {
	err := func() error {
		controlPlaneName = os.Getenv("CONTROLPLANE_NAME")
		managedClusterName = os.Getenv("MANAGED_CLUSTER_NAMESPACE")

		controlplaneConfig, err := clientcmd.BuildConfigFromFlags("", os.Getenv("CONTROLPLANE_KUBECONFIG"))
		if err != nil {
			return err
		}

		hubKubeClient, err = kubernetes.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

		clusterClient, err = clusterclient.NewForConfig(controlplaneConfig)
		if err != nil {
			return err
		}

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

		return nil
	}()

	gomega.Expect(err).ToNot(gomega.HaveOccurred())
})
