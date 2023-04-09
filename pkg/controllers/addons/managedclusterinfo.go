package addons

import (
	"context"

	"github.com/stolostron/multicloud-operators-foundation/pkg/controllers/clusterinfo"

	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// TODO update work manager to support this as option
const logCertSecret = "open-cluster-management/ocm-klusterlet-self-signed-secrets"

func SetupManagedClusterInfoWithManager(ctx context.Context, mgr manager.Manager) error {
	if err := clusterinfo.SetupWithManager(mgr, logCertSecret); err != nil {
		return err
	}

	return nil
}
