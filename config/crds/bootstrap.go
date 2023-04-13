// Copyright Contributors to the Open Cluster Management project
package crds

import (
	"context"
	"embed"
	"fmt"
	"sync"
	"time"

	crdhelpers "k8s.io/apiextensions-apiserver/pkg/apihelpers"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsapiserver "k8s.io/apiextensions-apiserver/pkg/apiserver"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apiextensionsv1client "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/typed/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
)

//go:embed *.yaml
var raw embed.FS

var addonCRDs = []string{
	// managed serviceaccount addon
	"0000_06_authentication.open-cluster-management.io_managedserviceaccounts.crd.yaml",
	// policy addon
	"0000_07_apps.open-cluster-management.io_placementrules.crd.yaml",
	"0000_07_policy.open-cluster-management.io_placementbindings.crd.yaml",
	"0000_07_policy.open-cluster-management.io_policies.crd.yaml",
	"0000_07_policy.open-cluster-management.io_policyautomations.crd.yaml",
	"0000_07_policy.open-cluster-management.io_policysets.crd.yaml",
	// managed cluster info
	"0000_08_internal.open-cluster-management.io_managedclusterinfos.crd.yaml",
}

func Bootstrap(ctx context.Context, crdClient apiextensionsclient.Interface) error {
	// poll here, call create to create base crds
	if err := wait.PollImmediateInfiniteWithContext(ctx, time.Second, func(ctx context.Context) (bool, error) {
		if err := CreateFromFile(ctx, crdClient.ApiextensionsV1().CustomResourceDefinitions(), addonCRDs, raw); err != nil {
			klog.Errorf("failed to bootstrap addon CRDs: %v", err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("failed to bootstrap addon CRDs: %w", err)
	}

	return nil
}

// CreateFromFile call createOneFromFile for each crd in filenames in parallel
func CreateFromFile(ctx context.Context, client apiextensionsv1client.CustomResourceDefinitionInterface, filenames []string, fs embed.FS) error {
	wg := sync.WaitGroup{}
	bootstrapErrChan := make(chan error, len(filenames))
	for _, fname := range filenames {
		wg.Add(1)
		go func(fn string) {
			defer wg.Done()
			err := retryError(func() error {
				return createOneFromFile(ctx, client, fn, fs)
			})

			if ctx.Err() != nil {
				err = ctx.Err()
			}
			bootstrapErrChan <- err
		}(fname)
	}
	wg.Wait()
	close(bootstrapErrChan)
	var bootstrapErrors []error
	for err := range bootstrapErrChan {
		bootstrapErrors = append(bootstrapErrors, err)
	}
	if err := utilerrors.NewAggregate(bootstrapErrors); err != nil {
		return fmt.Errorf("could not bootstrap CRDs: %w", err)
	}
	return nil
}

func createOneFromFile(ctx context.Context, client apiextensionsv1client.CustomResourceDefinitionInterface, filename string, fs embed.FS) error {
	start := time.Now()
	klog.V(4).Infof("Bootstrapping %v", filename)
	raw, err := fs.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("could not read CRD %s: %w", filename, err)
	}
	// set expected crd GVK
	expectedGvk := &schema.GroupVersionKind{Group: apiextensionsv1.GroupName, Version: "v1", Kind: "CustomResourceDefinition"}
	// read obj and gvk from file
	obj, gvk, err := extensionsapiserver.Codecs.UniversalDeserializer().Decode(raw, expectedGvk, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return fmt.Errorf("could not decode raw CRD %s: %w", filename, err)
	}
	if !equality.Semantic.DeepEqual(gvk, expectedGvk) {
		return fmt.Errorf("decoded CRD %s into incorrect GroupVersionKind, got %#v, wanted %#v", filename, gvk, expectedGvk)
	}
	// transform obj into type crd
	rawCrd, ok := obj.(*apiextensionsv1.CustomResourceDefinition)
	if !ok {
		return fmt.Errorf("decoded CRD %s into incorrect type, got %T, wanted %T", filename, rawCrd, &apiextensionsv1.CustomResourceDefinition{})
	}

	updateNeeded := false
	crd, err := client.Get(ctx, rawCrd.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			// if crd not found, create it
			crd, err = client.Create(ctx, rawCrd, metav1.CreateOptions{})
			if err != nil {
				// from KCP
				//
				// If multiple post-start hooks specify the same CRD, they could race with each other, so we need to
				// handle the scenario where another hook created this CRD after our Get() call returned not found.
				if apierrors.IsAlreadyExists(err) {
					// Re-get so we have the correct resourceVersion
					crd, err = client.Get(ctx, rawCrd.Name, metav1.GetOptions{})
					if err != nil {
						return fmt.Errorf("error getting CRD %s: %w", filename, err)
					}
					updateNeeded = true
				} else {
					return fmt.Errorf("error creating CRD %s: %w", filename, err)
				}
			} else {
				klog.Infof("Bootstrapped CRD %v after %s", filename, time.Since(start).String())
			}
		} else {
			return fmt.Errorf("error fetching CRD %s: %w", filename, err)
		}
	} else {
		updateNeeded = true
	}

	// if the version of already existing crd is not equal to the applying, update it
	if updateNeeded {
		rawCrd.ResourceVersion = crd.ResourceVersion
		_, err := client.Update(ctx, rawCrd, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
		klog.Infof("Updated CRD %v after %s", filename, time.Since(start).String())
	}

	// poll until crd condition is true
	return wait.PollImmediateInfiniteWithContext(ctx, 100*time.Millisecond, func(ctx context.Context) (bool, error) {
		crd, err := client.Get(ctx, rawCrd.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, fmt.Errorf("CRD %s was deleted before being established", filename)
			}
			return false, fmt.Errorf("error fetching CRD %s: %w", filename, err)
		}

		return crdhelpers.IsCRDConditionTrue(crd, apiextensionsv1.Established), nil
	})
}

func retryError(f func() error) error {
	return retry.OnError(retry.DefaultBackoff, func(err error) bool {
		return utilnet.IsConnectionRefused(err) || apierrors.IsTooManyRequests(err) || apierrors.IsConflict(err)
	}, f)
}

func WaitForOcmAddonCrdsReady(ctx context.Context, dynamicClient dynamic.Interface) bool {
	ocmAddonCrds := []string{
		// managed serviceaccount addon
		"managedserviceaccounts.authentication.open-cluster-management.io",
		// policy addon
		"placementrules.apps.open-cluster-management.io",
		"placementbindings.policy.open-cluster-management.io",
		"policies.policy.open-cluster-management.io",
		"policyautomations.policy.open-cluster-management.io",
		"policysets.policy.open-cluster-management.io",
		// managed cluster info
		"managedclusterinfos.internal.open-cluster-management.io",
	}
	klog.Infof("wait addon crds are installed")
	if err := wait.PollUntil(1*time.Second, func() (bool, error) {
		for _, crdName := range ocmAddonCrds {
			_, err := dynamicClient.Resource(schema.GroupVersionResource{
				Group:    apiextensionsv1.SchemeGroupVersion.Group,
				Version:  apiextensionsv1.SchemeGroupVersion.Version,
				Resource: "customresourcedefinitions",
			}).Get(ctx, crdName, metav1.GetOptions{})
			if err != nil {
				return false, nil
			}
			klog.Infof("addon crd(%s) is ready", crdName)
		}
		return true, nil
	}, ctx.Done()); err != nil {
		return false
	}
	klog.Infof("addon crds are ready")
	return true
}
