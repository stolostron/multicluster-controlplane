// Copyright Contributors to the Open Cluster Management project
package helpers

import (
	"context"
	"embed"
	"time"

	"github.com/openshift/library-go/pkg/assets"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"

	"k8s.io/apiextensions-apiserver/pkg/apihelpers"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	"open-cluster-management.io/multicluster-controlplane/pkg/util"
)

var (
	genericScheme = runtime.NewScheme()
	genericCodecs = serializer.NewCodecFactory(genericScheme)
	genericCodec  = genericCodecs.UniversalDeserializer()
)

func init() {
	utilruntime.Must(crdv1.AddToScheme(genericScheme))
}

func EnsureCRDs(ctx context.Context, client apiextensionsclient.Interface, fs embed.FS, crds ...string) error {
	return wait.PollImmediate(1*time.Second, 5*time.Minute, func() (bool, error) {
		for _, crdFileName := range crds {
			klog.Infof("waiting for crd %s", crdFileName)
			template, err := fs.ReadFile(crdFileName)
			utilruntime.Must(err)

			objData := assets.MustCreateAssetFromTemplate(crdFileName, template, nil).Data
			obj, _, err := genericCodec.Decode(objData, nil, nil)
			utilruntime.Must(err)

			switch required := obj.(type) {
			case *crdv1.CustomResourceDefinition:
				crd, _, err := resourceapply.ApplyCustomResourceDefinitionV1(
					ctx,
					client.ApiextensionsV1(),
					util.NewLoggingRecorder("crd-generator"),
					required,
				)
				if err != nil {
					klog.Errorf("fail to apply %s due to %v", crdFileName, err)
					return false, nil
				}

				if !apihelpers.IsCRDConditionTrue(crd, crdv1.Established) {
					return false, nil
				}

				klog.Infof("crd %s is ready", crd.Name)
			}
		}

		return true, nil
	})
}
