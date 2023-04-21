// Copyright Contributors to the Open Cluster Management project
package e2e_test

import (
	"fmt"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const expectedClusters = 2

var _ = ginkgo.Describe("ManagedCluster", func() {
	ginkgo.It("all managed clusters should be available", func() {
		gomega.Eventually(func() error {
			clusters, err := clusterClient.ClusterV1().ManagedClusters().List(ctx, metav1.ListOptions{})
			if err != nil {
				return err
			}
			if len(clusters.Items) != expectedClusters {
				return fmt.Errorf("expected %d clusters, but get %+v", expectedClusters, clusters.Items)
			}

			// each cluster should be available
			managedCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, fmt.Sprintf("%s-mc", controlPlaneName), metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
				return fmt.Errorf("expected %s is available, but failed", fmt.Sprintf("%s-mc", controlPlaneName))
			}

			hostedCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, fmt.Sprintf("%s-hosted-mc", controlPlaneName), metav1.GetOptions{})
			if err != nil {
				return err
			}
			if !meta.IsStatusConditionTrue(hostedCluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
				return fmt.Errorf("expected %s is available, but failed", fmt.Sprintf("%s-hosted-mc", controlPlaneName))
			}

			return nil
		}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
	})
})
