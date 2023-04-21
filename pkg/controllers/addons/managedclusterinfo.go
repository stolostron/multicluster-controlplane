package addons

import (
	"context"

	"github.com/stolostron/multicloud-operators-foundation/pkg/controllers/clusterinfo"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

func SetupManagedClusterInfoWithManager(ctx context.Context, mgr manager.Manager) error {
	if err := clusterinfo.SetupWithManager(mgr, ""); err != nil {
		return err
	}

	return nil
}
