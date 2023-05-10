package e2e_test

import (
	"fmt"
	"os"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	clusterinfov1beta1 "github.com/stolostron/cluster-lifecycle-api/clusterinfo/v1beta1"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	msav1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"

	"github.com/stolostron/multicluster-controlplane/test/e2e/util"
)

const msaName = "msa-e2e"

var _ = ginkgo.Describe("default mode loopback test", func() {
	var managedClusterName string

	ginkgo.BeforeEach(func() {
		managedClusterName = os.Getenv("MANAGED_CLUSTER")

		ginkgo.By(fmt.Sprintf("Wait for managed cluster %s available", managedClusterName), func() {
			gomega.Eventually(func() error {
				managedCluster, err := controlplaneClients.clusterClient.ClusterV1().ManagedClusters().Get(ctx, managedClusterName, metav1.GetOptions{})
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

	ginkgo.Context("manifestworks should work fine", func() {
		ginkgo.It("should be able to create/delete manifestoworks", func() {
			workName := fmt.Sprintf("%s-%s", managedClusterName, rand.String(6))
			configMapName := fmt.Sprintf("%s-%s", managedClusterName, rand.String(6))

			ginkgo.By(fmt.Sprintf("Create a manifestwork %q in the cluster %q", workName, managedClusterName), func() {
				_, err := controlplaneClients.workClient.WorkV1().ManifestWorks(managedClusterName).Create(
					ctx,
					&workv1.ManifestWork{
						ObjectMeta: metav1.ObjectMeta{
							Name: workName,
						},
						Spec: workv1.ManifestWorkSpec{
							Workload: workv1.ManifestsTemplate{
								Manifests: []workv1.Manifest{
									util.ToManifest(util.NewConfigmap(configMapName)),
								},
							},
						},
					},
					metav1.CreateOptions{},
				)
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Waiting the manifestwork becomes available", func() {
				gomega.Expect(wait.Poll(1*time.Second, timeout, func() (bool, error) {
					work, err := controlplaneClients.workClient.WorkV1().ManifestWorks(managedClusterName).Get(ctx, workName, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						return false, nil
					}
					if err != nil {
						return false, err
					}

					if meta.IsStatusConditionTrue(work.Status.Conditions, workv1.WorkAvailable) {
						return true, nil
					}

					return false, nil
				})).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Get the configmap that was created by manifestwork", func() {
				_, err := spokeClients.kubeClient.CoreV1().ConfigMaps(util.DefaultNamespace).Get(ctx, configMapName, metav1.GetOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Delete the manifestwork", func() {
				err := controlplaneClients.workClient.WorkV1().ManifestWorks(managedClusterName).Delete(ctx, workName, metav1.DeleteOptions{})
				gomega.Expect(err).ToNot(gomega.HaveOccurred())
			})

			ginkgo.By("Waiting the configmap is deleted on the managed cluster", func() {
				gomega.Expect(wait.Poll(1*time.Second, timeout, func() (bool, error) {
					_, err := spokeClients.kubeClient.CoreV1().ConfigMaps(util.DefaultNamespace).Get(ctx, configMapName, metav1.GetOptions{})
					if errors.IsNotFound(err) {
						return true, nil
					}
					if err != nil {
						return false, err
					}

					return false, nil
				})).ToNot(gomega.HaveOccurred())
			})
		})
	})

	ginkgo.Context("work manager should work fine", func() {
		ginkgo.It("should have a synced clusterinfo", func() {
			gomega.Eventually(func() error {
				clusterInfo, err := util.GetResource(ctx, controlplaneClients.dynamicClient, util.ClusterInfoGVR, managedClusterName, managedClusterName)
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
				if _, err := spokeClients.clusterClient.ClusterV1alpha1().ClusterClaims().Get(ctx, util.IDClaim, metav1.GetOptions{}); err != nil {
					return err
				}

				if _, err := spokeClients.clusterClient.ClusterV1alpha1().ClusterClaims().Get(ctx, util.VersionClaim, metav1.GetOptions{}); err != nil {
					return err
				}

				claimNames, err := util.GetManagedClusterClaims(ctx, controlplaneClients.clusterClient, managedClusterName)
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
				err := controlplaneClients.saClient.Authentication().ManagedServiceAccounts(managedClusterName).Delete(ctx, msaName, metav1.DeleteOptions{})
				gomega.Expect(err).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Reported secret should be deleted on hub cluster", func() {
				gomega.Eventually(func() bool {
					_, err := controlplaneClients.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
					return errors.IsNotFound(err)
				}, timeout, time.Second).Should(gomega.BeTrue())
			})

			ginkgo.By("ServiceAccount should be deleted on managed cluster", func() {
				gomega.Eventually(func() bool {
					_, err := spokeClients.kubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
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

				_, err := controlplaneClients.saClient.Authentication().ManagedServiceAccounts(managedClusterName).Create(ctx, msa, metav1.CreateOptions{})
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
			})

			ginkgo.By("Check the ServiceAccount on the managed cluster", func() {
				gomega.Eventually(func() error {
					_, err := spokeClients.kubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					return nil
				}).WithTimeout(addonTimeout).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Validate the status of ManagedServiceAccount", func() {
				gomega.Eventually(func() error {
					msa, err := controlplaneClients.saClient.Authentication().ManagedServiceAccounts(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
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
					msa, err := controlplaneClients.saClient.Authentication().ManagedServiceAccounts(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					secret, err := controlplaneClients.kubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msa.Status.TokenSecretRef.Name, metav1.GetOptions{})
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

					tr, err := spokeClients.kubeClient.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
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
			ginkgo.By("Verify the policy is propagated to the managed cluster", func() {
				gomega.Eventually(func() error {
					_, err := controlplaneClients.dynamicClient.Resource(policyv1.GroupVersion.WithResource("policies")).
						Namespace(managedClusterName).Get(ctx, policyNamespace+"."+policyName, metav1.GetOptions{})
					if err != nil {
						return err
					}
					return nil
				}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Verify the policy is compliant", func() {
				gomega.Eventually(func() error {
					var err error
					policy, err := controlplaneClients.dynamicClient.Resource(policyv1.GroupVersion.WithResource("policies")).
						Namespace(managedClusterName).Get(ctx, policyNamespace+"."+policyName, metav1.GetOptions{})
					if err != nil {
						return err
					}

					statusObj, ok := policy.Object["status"]
					if ok {
						status := statusObj.(map[string]interface{})
						if status["compliant"] == "Compliant" {
							return nil
						}
					}
					return fmt.Errorf("policy is not compliant")
				}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
			})

			ginkgo.By("Verify the policy is enforced to the managed cluster", func() {
				gomega.Eventually(func() error {
					_, err := spokeClients.kubeClient.CoreV1().LimitRanges(policyNamespace).
						Get(ctx, limitrangeName, metav1.GetOptions{})
					if err != nil {
						return err
					}
					return nil
				}).WithTimeout(timeout).ShouldNot(gomega.HaveOccurred())
			})
		})
	})

})
