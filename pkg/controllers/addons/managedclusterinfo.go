package addons

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/stolostron/multicluster-controlplane/pkg/controllers/addons/managedclusterinfo"
)

func SetupManagedClusterInfoWithManager(ctx context.Context, mgr manager.Manager) error {
	if err := managedclusterinfo.SetupWithManager(mgr, ""); err != nil {
		return err
	}

	return nil
}
