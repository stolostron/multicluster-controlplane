// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"fmt"
	"os"
	"strings"

	"github.com/stolostron/multicluster-controlplane/pkg/constants"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"
)

// ContainsString to check string from a slice of strings.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// ContainsString to remove string from a slice of strings.
func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func ClusterIsOffLine(conditions []metav1.Condition) bool {
	return meta.IsStatusConditionPresentAndEqual(conditions, clusterapiv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown)
}

func GetImage(imageEnvName, defaultImage string) string {
	image := os.Getenv(imageEnvName)
	if image == "" {
		image = defaultImage
	}

	// the image has tag or digest, return it directly
	if strings.Contains(image, ":") || strings.Contains(image, "@") {
		return image
	}

	snapshotVersion := os.Getenv(constants.SnapshotVersionEnvName)
	if snapshotVersion == "" {
		snapshotVersion = constants.DefaultSnapshotVersion
	}

	return fmt.Sprintf("%s:%s", image, snapshotVersion)
}
