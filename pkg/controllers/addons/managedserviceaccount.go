package addons

import (
	"context"

	"github.com/stolostron/multicluster-controlplane/pkg/feature"

	"open-cluster-management.io/managed-serviceaccount/pkg/addon/manager"
	"open-cluster-management.io/multicluster-controlplane/pkg/features"

	ctrl "sigs.k8s.io/controller-runtime"
)

func SetupManagedServiceAccountWithManager(ctx context.Context, mgr ctrl.Manager) error {
	if features.DefaultControlplaneMutableFeatureGate.Enabled(feature.ManagedServiceAccountEphemeralIdentity) {
		ctrl := manager.NewEphemeralIdentityReconciler(mgr.GetCache(), mgr.GetClient())
		if err := ctrl.SetupWithManager(mgr); err != nil {
			return err
		}
	}

	return nil
}
