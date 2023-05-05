package util

import (
	"context"
	"os/exec"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/dynamic"

	clusterclient "open-cluster-management.io/api/client/cluster/clientset/versioned"
	clusterv1alpha1 "open-cluster-management.io/api/cluster/v1alpha1"
	workv1 "open-cluster-management.io/api/work/v1"
)

const (
	IDClaim          = "id.k8s.io"
	VersionClaim     = "kubeversion.open-cluster-management.io"
	DefaultNamespace = "default"
)

var (
	ClusterInfoGVR = schema.GroupVersionResource{
		Group:    "internal.open-cluster-management.io",
		Version:  "v1beta1",
		Resource: "managedclusterinfos",
	}
)

func GetResource(ctx context.Context, dynamicClient dynamic.Interface, gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error) {
	obj, err := dynamicClient.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func IsResourceStatusConditionTrue(obj *unstructured.Unstructured, conditionType string) bool {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil {
		return false
	}

	if !found {
		return false
	}

	for _, condition := range conditions {
		conditionValue, ok := condition.(map[string]interface{})
		if !ok {
			return false
		}

		if conditionValue["type"] == conditionType {
			return conditionValue["status"] == "True"
		}
	}

	return false
}

func GetManagedClusterClaims(ctx context.Context, clusterClient clusterclient.Interface, clusterName string) (sets.Set[string], error) {
	claimNames := sets.New[string]()
	managedCluster, err := clusterClient.ClusterV1().ManagedClusters().Get(ctx, clusterName, metav1.GetOptions{})
	if err != nil {
		return claimNames, err
	}

	for _, claim := range managedCluster.Status.ClusterClaims {
		claimNames.Insert(claim.Name)
	}

	return claimNames, nil
}

func NewClaim() *clusterv1alpha1.ClusterClaim {
	return &clusterv1alpha1.ClusterClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: rand.String(6),
		},
		Spec: clusterv1alpha1.ClusterClaimSpec{
			Value: rand.String(6),
		},
	}
}

func Kubectl(kubeConfig string, args ...string) (string, error) {
	args = append([]string{"--kubeconfig", kubeConfig}, args...)
	output, err := exec.Command("kubectl", args...).CombinedOutput()
	return string(output), err
}

func ToManifest(object runtime.Object) workv1.Manifest {
	manifest := workv1.Manifest{}
	manifest.Object = object
	return manifest
}

func NewConfigmap(name string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: DefaultNamespace,
			Name:      name,
		},
		Data: map[string]string{
			"test": "I'm a test configmap",
		},
	}
}
