// Copyright Contributors to the Open Cluster Management project
package e2e_test

import (
	"fmt"
	"time"

	ginkgo "github.com/onsi/ginkgo/v2"
	gomega "github.com/onsi/gomega"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	msav1alpha1 "open-cluster-management.io/managed-serviceaccount/api/v1alpha1"
)

const msaName = "msa-e2e"

var _ = ginkgo.Describe("ManagedServiceAccount", func() {
	ginkgo.It("token projection should work", func() {
		ginkgo.By("Create a ManagedServiceAccount on the hub cluster")
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

		ginkgo.By("Check the ServiceAccount on the managed cluster")
		gomega.Eventually(func() error {
			_, err := spokeKubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			return nil
		}).WithTimeout(addonTimeout).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Validate the status of ManagedServiceAccount")
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

		ginkgo.By("Validate the reported token")
		gomega.Eventually(func() error {
			msa, err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
			if err != nil {
				return err
			}

			secret, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msa.Status.TokenSecretRef.Name, metav1.GetOptions{})
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

	ginkgo.AfterEach(func() {
		ginkgo.By("Delete the ManagedServiceAccount")
		err := saClient.Authentication().ManagedServiceAccounts(managedClusterName).Delete(ctx, msaName, metav1.DeleteOptions{})
		gomega.Expect(err).ShouldNot(gomega.HaveOccurred())

		ginkgo.By("Reported secret should be deleted on hub cluster")
		gomega.Eventually(func() bool {
			_, err := hubKubeClient.CoreV1().Secrets(managedClusterName).Get(ctx, msaName, metav1.GetOptions{})
			return errors.IsNotFound(err)
		}, timeout, time.Second).Should(gomega.BeTrue())

		ginkgo.By("ServiceAccount should be deleted on managed cluster")
		gomega.Eventually(func() bool {
			_, err := spokeKubeClient.CoreV1().ServiceAccounts(agentNamespace).Get(ctx, msaName, metav1.GetOptions{})
			return errors.IsNotFound(err)
		}, timeout, time.Second).Should(gomega.BeTrue())
	})
})
