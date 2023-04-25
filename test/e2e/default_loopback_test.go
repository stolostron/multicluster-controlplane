package e2e_test

import (
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	msav1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"

	"github.com/stolostron/multicluster-controlplane/test/e2e/util"
)

const msaName = "msa-e2e"

var _ = ginkgo.Describe("default mode loopback test", func() {
	ginkgo.BeforeEach(func() {
		ginkgo.By(fmt.Sprintf("Wait for managed cluster %s available", managedClusterName), func() {
			gomega.Eventually(func() error {
				managedCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, managedClusterName, metav1.GetOptions{})
				if err != nil {
					return err
				}

				if !meta.IsStatusConditionTrue(managedCluster.Status.Conditions, clusterv1.ManagedClusterConditionAvailable) {
					return fmt.Errorf("expected cluster %s is available, but failed", managedClusterName)
				}

				return nil
			}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.Context("work manager should work fine", func() {
		ginkgo.It("should have a synced clusterinfo", func() {
			gomega.Eventually(func() error {
				clusterInfo, err := util.GetResource(ctx, dynamicClient, util.ClusterInfoGVR, managedClusterName, managedClusterName)
				if err != nil {
					return err
				}

				if !util.IsResourceStatusConditionTrue(clusterInfo, clusterinfov1beta1.ManagedClusterInfoSynced) {
					return fmt.Errorf("expected clusterinfo %s is synced, but failed", managedClusterName)
				}

				return nil
			}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
		})

		ginkgo.It("should have required claims", func() {
			gomega.Eventually(func() error {
				if _, err := spokeClusterClient.ClusterV1alpha1().ClusterClaims().Get(ctx, util.IDClaim, metav1.GetOptions{}); err != nil {
					return err
				}

				if _, err := spokeClusterClient.ClusterV1alpha1().ClusterClaims().Get(ctx, util.VersionClaim, metav1.GetOptions{}); err != nil {
					return err
				}

				claimNames, err := util.GetManagedClusterClaims(ctx, clusterClient, managedClusterName)
				if err != nil {
					return err
				}

				if !claimNames.Has(util.IDClaim) {
					return fmt.Errorf("claim %q is not reported", util.IDClaim)
				}

				if !claimNames.Has(util.VersionClaim) {
					return fmt.Errorf("claim %q is not reported", util.VersionClaim)
				}

				return nil
			}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
		})
	})

	ginkgo.Context("managed serviceaccount should work fine", func() {
		ginkgo.AfterEach(func() {
			ginkgo.By("Delete the ManagedServiceAccount", func() {
				err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Delete(ctx, msaName, metav1.DeleteOptions{})
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Reported secret should be deleted on hub cluster", func() {
				gomega.Eventually(func() bool {
					_, err := kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
					return errors.IsNotFound(err)
				}, timeout, time.Second).Should(gomega.BeTrue())
			})

			ginkgo.By("ServiceAccount should be deleted on managed cluster", func() {
				gomega.Eventually(func() bool {
					_, err := spokeKubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
					return errors.IsNotFound(err)
				}, timeout, time.Second).Should(gomega.BeTrue())
			})
		})

		ginkgo.It("should report a valid token from managed cluster", func() {
			ginkgo.By("Create a ManagedServiceAccount on the hub cluster", func() {
				msa := &msav1alpha1.ManagedServiceAccount{
					ObjectMeta: metav1.ObjectMeta{
						Name: msaName,
					},
					Spec: msav1alpha1.ManagedServiceAccountSpec{
						Rotation: msav1alpha1.ManagedServiceAccountRotation{
							Enabled:  true,
							Validity: metav1.Duration{Duration: time.Minute * 30},
						},
					},
				}

				_, err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Create(ctx, msa, metav1.CreateOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})

			ginkgo.By("Check the ServiceAccount on the managed cluster", func() {
				gomega.Eventually(func() error {
					_, err := spokeKubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					return nil
				}).WithTimeout(addonTimeout).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Validate the status of ManagedServiceAccount", func() {
				gomega.Eventually(func() error {
					msa, err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					if !meta.IsStatusConditionTrue(msa.Status.Conditions, msav1alpha1.ConditionTypeSecretCreated) {
						return fmt.Errorf("the secret: %s/%s has not been created in hub", managedClusterName, msaName)
					}

					if !meta.IsStatusConditionTrue(msa.Status.Conditions, msav1alpha1.ConditionTypeTokenReported) {
						return fmt.Errorf("the token has not been reported to secret: %s/%s", managedClusterName, msaName)
					}

					if msa.Status.TokenSecretRef == nil {
						return fmt.Errorf("the ManagedServiceAccount not associated any token secret")
					}

					return nil
				}).WithTimeout(addonTimeout).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Validate the reported token", func() {
				gomega.Eventually(func() error {
					msa, err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					secret, err := kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msa.Status.TokenSecretRef.Name, metav1.GetOptions{})
					if err != nil {
						return err
					}

					token := secret.Data[corev1.ServiceAccountTokenKey]
					tokenReview := &authv1.TokenReview{
						TypeMeta: metav1.TypeMeta{
							Kind:       "TokenReview",
							APIVersion: "authentication.k8s.io/v1",
						},
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "token-review-request",
						},
						Spec: authv1.TokenReviewSpec{
							Token: string(token),
						},
					}

					tr, err := spokeKubeClient.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
					if err != nil {
						return err
					}

					if !tr.Status.Authenticated {
						return fmt.Errorf("the secret: %s/%s token should be authenticated by the managed cluster service account", secret.GetNamespace(), secret.GetName())
					}

					return nil
				}).WithTimeout(addonTimeout).ShouldNot(gomega.HaveOccurred())
			})
		})
	})

	ginkgo.Context("policy should work fine", func() {
		ginkgo.It("should be able to propagate policies", func() {
			ginkgo.By("TODO", func() {})
		})
	})

})
