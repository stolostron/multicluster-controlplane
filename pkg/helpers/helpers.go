// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"context"
	"embed"
	"os"
	"time"

	"github.com/openshift/library-go/pkg/assets"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"

	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	clusterapiv1 "open-cluster-management.io/api/cluster/v1"

	"open-cluster-management.io/multicluster-controlplane/pkg/util"
)

func EnsureCRDs(ctx context.Context, scheme *runtime.Scheme, client apiextensionsclient.Interface, fs embed.FS, crds ...string) error {
	crdMap := make(map[string]*crdv1.CustomResourceDefinition, len(crds))
	for _, crdFileName := range crds {
		klog.V(4).Infof("waiting for crd %s", crdFileName)
		template, err := fs.ReadFile(crdFileName)
		if err != nil {
			return err
		}

		objData := assets.MustCreateAssetFromTemplate(crdFileName, template, nil).Data
		obj, _, err := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode(objData, nil, nil)
		if err != nil {
			return err
		}

		switch required := obj.(type) {
		case *crdv1.CustomResourceDefinition:
			crdMap[crdFileName] = required
		}
	}

	return wait.PollUntilContextCancel(ctx, 1*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, crdFileName := range crds {
			klog.V(4).Infof("waiting for crd %s", crdFileName)
			if crdObj, ok := crdMap[crdFileName]; ok && crdObj != nil {
				crd, _, err := resourceapply.ApplyCustomResourceDefinitionV1(
					ctx,
					client.ApiextensionsV1(),
					util.NewLoggingRecorder("crd-generator"),
					crdObj,
				)
				if err != nil {
					klog.Errorf("fail to apply %s due to %v", crdFileName, err)
					return false, nil
				}

				if !apihelpers.IsCRDConditionTrue(crd, crdv1.Established) {
					return false, nil
				}

				klog.Infof("crd %s is ready", crd.Name)
				// reset crd opinter to nil to avoid duplicated apply
				crdMap[crdFileName] = nil
			}
		}

		return true, nil
	})
}

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func RemoveString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}

func GetComponentNamespace() (string, error) {
	nsBytes, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		return "open-cluster-management-agent-addon", err
	}
	return string(nsBytes), nil
}

func ClusterIsOffLine(conditions []metav1.Condition) bool {
	return meta.IsStatusConditionPresentAndEqual(conditions, clusterapiv1.ManagedClusterConditionAvailable, metav1.ConditionUnknown)
}
