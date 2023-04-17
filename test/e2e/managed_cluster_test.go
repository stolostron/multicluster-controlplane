// Copyright Contributors to the Open Cluster Management project
package e2e_test

import (
	"context"
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	clusterv1 "open-cluster-management.io/api/cluster/v1"
)

const expectedClusters = 2

var _ = ginkgo.Describe("ManagedCluster", ginkgo.Label("cluster"), func() {
	ginkgo.It("all managed clusters should be available from all controlplanes", func() {
		gomega.Eventually(func() error {
			for _, controlPlane := range options.ControlPlanes {
				client := runtimeClientMap[controlPlane.Name]

				// each controlplane should have `expectedClusters` clusters
				clusters := &clusterv1.ManagedClusterList{}
				if err := client.List(context.TODO(), clusters); err != nil {
					return err
				}
				if len(clusters.Items) != expectedClusters {
					return fmt.Errorf("expected %d clusters, but get %+v", expectedClusters, clusters.Items)
				}

				// each cluster should be available
				managedCluster := clusterv1.ManagedCluster{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-mc1", controlPlane.Name)}, &managedCluster); err != nil {
					return err
				}
				if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
					return fmt.Errorf("expected %s is available, but failed", fmt.Sprintf("%s-mc1", controlPlane.Name))
				}

				hostedManagedCluster := clusterv1.ManagedCluster{}
				if err := client.Get(context.TODO(), types.NamespacedName{Name: fmt.Sprintf("%s-hosted-mc1", controlPlane.Name)}, &hostedManagedCluster); err != nil {
					return err
				}
				if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
					return fmt.Errorf("expected %s is available, but failed", fmt.Sprintf("%s-hosted-mc1", controlPlane.Name))
				}
			}

			return nil
		}).WithTimeout(90 * time.Second).ShouldNot(gomega.HaveOccurred())
	})
})
