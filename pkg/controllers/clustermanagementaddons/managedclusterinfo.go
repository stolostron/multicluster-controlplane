// Copyright Contributors to the Open Cluster Management project

package clustermanagementaddons

import (
	"github.com/stolostron/multicloud-operators-foundation/pkg/controllers/clusterinfo"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	logCertSecret = "open-cluster-management/ocm-klusterlet-self-signed-secrets"
)

func SetupClusterInfoWithManager(mgr manager.Manager) error {
	if err := clusterinfo.SetupWithManager(mgr, logCertSecret); err != nil {
		return err
	}
	return nil
}
