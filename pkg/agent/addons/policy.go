package addons

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openshift/library-go/pkg/controller/factory"
	depclient "github.com/stolostron/kubernetes-dependency-watches/client"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	kubecache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	configpolicyv1 "open-cluster-management.io/config-policy-controller/api/v1"
	"open-cluster-management.io/config-policy-controller/controllers"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/specsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/statussync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/templatesync"
	policyv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	"open-cluster-management.io/multicluster-controlplane/pkg/util"

	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var policiesv1APIVersion = policyv1.SchemeGroupVersion.Group + "/" + policyv1.SchemeGroupVersion.Version

var correlatorOptions = record.CorrelatorOptions{
	// This essentially disables event aggregation of the same events but with different messages.
	MaxIntervalInSeconds: 1,
	// This is the default spam key function except it adds the reason and message as well.
	// https://github.com/kubernetes/client-go/blob/v0.23.3/tools/record/events_cache.go#L70-L82
	SpamKeyFunc: func(event *corev1.Event) string {
		return strings.Join(
			[]string{
				event.Source.Component,
				event.Source.Host,
				event.InvolvedObject.Kind,
				event.InvolvedObject.Namespace,
				event.InvolvedObject.Name,
				string(event.InvolvedObject.UID),
				event.InvolvedObject.APIVersion,
				event.Reason,
				event.Message,
			},
			"",
		)
	},
}

type PolicyAgentConfig struct {
	DecryptionConcurrency uint8
	EvaluationConcurrency uint8
	EnableMetrics         bool
	Frequency             uint
}

func StartPolicyAgentWithCache(ctx context.Context,
	clusterName string,
	scheme *runtime.Scheme,
	hubKubeConfig, hostingKubeConfig, spokeKubeConfig *rest.Config,
	hubCache, hostingCache cache.Cache,
	hubClient, hostingClient client.Client,
	config *PolicyAgentConfig) error {
	instanceName, _ := os.Hostname() // on an error, instanceName will be empty, which is ok

	hubKubeClient, err := kubernetes.NewForConfig(hubKubeConfig)
	if err != nil {
		return err
	}

	hostingKubeClient, err := kubernetes.NewForConfig(hostingKubeConfig)
	if err != nil {
		return err
	}

	spokeKubeClient, err := kubernetes.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	spokeDynamicClient, err := dynamic.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	// create target namespace if it doesn't exist
	targetNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}
	_, err = hostingKubeClient.CoreV1().Namespaces().Create(ctx, targetNamespace, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	hubEventBroadcaster := record.NewBroadcasterWithCorrelatorOptions(correlatorOptions)
	hubEventBroadcaster.StartRecordingToSink(
		&clientcorev1.EventSinkImpl{Interface: hubKubeClient.CoreV1().Events(clusterName)},
	)

	hostingEventBroadcaster := record.NewBroadcasterWithCorrelatorOptions(correlatorOptions)
	hostingEventBroadcaster.StartRecordingToSink(
		&clientcorev1.EventSinkImpl{Interface: hostingKubeClient.CoreV1().Events(clusterName)},
	)

	spokeEventBroadcaster := record.NewBroadcasterWithCorrelatorOptions(correlatorOptions)
	spokeEventBroadcaster.StartRecordingToSink(
		&clientcorev1.EventSinkImpl{Interface: spokeKubeClient.CoreV1().Events(clusterName)},
	)

	// watch hub policy
	policyReconciler := &specsync.PolicyReconciler{
		HubClient:       hubClient,
		ManagedClient:   hostingClient,
		Scheme:          scheme,
		TargetNamespace: clusterName,
		ManagedRecorder: spokeEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: specsync.ControllerName}),
	}

	// watch hub secret
	secretReconciler := &secretsync.SecretReconciler{
		Client:          hubClient,
		ManagedClient:   hostingClient,
		Scheme:          scheme,
		TargetNamespace: clusterName,
	}

	// watch hosting configurationpolicy
	configurationPolicyReconciler := &controllers.ConfigurationPolicyReconciler{
		Client:                 hostingClient,
		DecryptionConcurrency:  config.DecryptionConcurrency,
		EvaluationConcurrency:  config.EvaluationConcurrency,
		Scheme:                 scheme,
		InstanceName:           instanceName,
		TargetK8sClient:        spokeKubeClient,
		TargetK8sDynamicClient: spokeDynamicClient,
		TargetK8sConfig:        spokeKubeConfig,
		EnableMetrics:          config.EnableMetrics,
		Recorder:               hostingEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: controllers.ControllerName}),
	}

	// watch policies and events on the hosting cluster
	hostingPolicyReconciler := &statussync.PolicyReconciler{
		ClusterNamespaceOnHub: clusterName,
		HubClient:             hubClient,
		ManagedClient:         hostingClient,
		Scheme:                scheme,
		HubRecorder:           hubEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: statussync.ControllerName}),
		ManagedRecorder:       hostingEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: statussync.ControllerName}),
	}

	depReconciler, depEvents := depclient.NewControllerRuntimeSource()
	watcher, err := depclient.New(hostingKubeConfig, depReconciler, nil)
	if err != nil {
		return err
	}

	templateReconciler := &templatesync.PolicyReconciler{
		Client:           hostingClient,
		DynamicWatcher:   watcher,
		Scheme:           scheme,
		Config:           hostingKubeConfig,
		ClusterNamespace: clusterName,
		Recorder:         hostingEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: templatesync.ControllerName}),
		Clientset:        kubernetes.NewForConfigOrDie(hostingKubeConfig),
		InstanceName:     instanceName,
	}

	go func() {
		utilruntime.Must(watcher.Start(ctx))
	}()

	// Wait until the dynamic watcher has started.
	<-watcher.Started()

	policyCtrl := newPolicyController(ctx, policyReconciler, hubCache)
	secretCtrl := newSecretController(ctx, clusterName, secretReconciler, hubCache)
	configPolicyCtrl := newConfigurationPolicyController(ctx, configurationPolicyReconciler, hostingCache)
	hostingPolicyCtrl := newHostingPolicyController(ctx, hostingPolicyReconciler, templateReconciler, hostingCache, depEvents)

	go hubCache.Start(ctx)
	go hostingCache.Start(ctx)

	go policyCtrl.Run(ctx, 1)
	go secretCtrl.Run(ctx, 1)
	go configPolicyCtrl.Run(ctx, 1)
	go hostingPolicyCtrl.Run(ctx, 1)

	elected := make(chan struct{})
	go func() {
		// starting the `PeriodicallyExecConfigPolicies` after 10 seconds to avoid the cache is not started
		time.Sleep(10 * time.Second)
		close(elected)
	}()

	go func() {
		configurationPolicyReconciler.PeriodicallyExecConfigPolicies(ctx, config.Frequency, elected)
	}()

	return nil
}

type policyController struct {
	policyReconciler *specsync.PolicyReconciler
	hubCache         cache.Cache
}

func newPolicyController(
	ctx context.Context,
	policyReconciler *specsync.PolicyReconciler,
	hubCache cache.Cache) factory.Controller {
	controller := &policyController{
		policyReconciler: policyReconciler,
		hubCache:         hubCache,
	}

	policyInformer, err := hubCache.GetInformer(ctx, &policyv1.Policy{})
	utilruntime.Must(err)

	return factory.New().WithSync(controller.sync).
		WithInformersQueueKeyFunc(func(obj runtime.Object) string {
			key, _ := kubecache.MetaNamespaceKeyFunc(obj)
			return key
		}, policyInformer).
		ToController("PolicyController", util.NewLoggingRecorder("policy-controller"))
}

func (c *policyController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	queueKey := controllerContext.QueueKey()
	if queueKey == factory.DefaultQueueKey {
		// triggered by resync, requeue all objects
		policies := &policyv1.PolicyList{}
		if err := c.hubCache.List(ctx, policies); err != nil {
			return err
		}

		for _, policy := range policies.Items {
			controllerContext.Queue().Add(fmt.Sprintf("%s/%s", policy.Namespace, policy.Name))
		}
		return nil
	}

	namespace, name, err := kubecache.SplitMetaNamespaceKey(queueKey)
	utilruntime.HandleError(err)

	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	_, err = c.policyReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
	return err
}

type secretController struct {
	clusterName      string
	secretReconciler *secretsync.SecretReconciler
}

func newSecretController(
	ctx context.Context,
	clusterName string,
	secretReconciler *secretsync.SecretReconciler,
	hubCache cache.Cache) factory.Controller {
	controller := &secretController{
		clusterName:      clusterName,
		secretReconciler: secretReconciler,
	}

	secretInformer, err := hubCache.GetInformer(ctx, &corev1.Secret{})
	utilruntime.Must(err)

	return factory.New().WithSync(controller.sync).
		WithFilteredEventsInformersQueueKeyFunc(func(obj runtime.Object) string {
			key, _ := kubecache.MetaNamespaceKeyFunc(obj)
			return key
		}, func(obj interface{}) bool {
			metaObj, ok := obj.(metav1.ObjectMetaAccessor)
			if !ok {
				return false
			}
			if metaObj.GetObjectMeta().GetNamespace() != clusterName {
				return false
			}
			if metaObj.GetObjectMeta().GetName() != secretsync.SecretName {
				return false
			}
			return false
		}, secretInformer).
		ToController("PolicySecretController", util.NewLoggingRecorder("policy-secret-controller"))
}

func (c *secretController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	queueKey := controllerContext.QueueKey()
	if queueKey == factory.DefaultQueueKey {
		// triggered by resync, requeue the object
		controllerContext.Queue().Add(fmt.Sprintf("%s/%s", c.clusterName, secretsync.SecretName))
		return nil
	}

	namespace, name, err := kubecache.SplitMetaNamespaceKey(queueKey)
	utilruntime.HandleError(err)

	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	_, err = c.secretReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
	return err
}

type configurationPolicyController struct {
	configurationPolicyReconciler *controllers.ConfigurationPolicyReconciler
	hostingCache                  cache.Cache
}

func newConfigurationPolicyController(
	ctx context.Context,
	configurationPolicyReconciler *controllers.ConfigurationPolicyReconciler,
	hostingCache cache.Cache) factory.Controller {
	controller := &configurationPolicyController{
		configurationPolicyReconciler: configurationPolicyReconciler,
		hostingCache:                  hostingCache,
	}

	configurationPolicyInformer, err := hostingCache.GetInformer(ctx, &configpolicyv1.ConfigurationPolicy{})
	utilruntime.Must(err)

	return factory.New().WithSync(controller.sync).
		WithInformersQueueKeyFunc(func(obj runtime.Object) string {
			key, _ := kubecache.MetaNamespaceKeyFunc(obj)
			return key
		}, configurationPolicyInformer).
		ToController("ConfigurationPolicyController", util.NewLoggingRecorder("configurationpolicy-controller"))
}

func (c *configurationPolicyController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	queueKey := controllerContext.QueueKey()

	if queueKey == factory.DefaultQueueKey {
		// triggered by resync, requeue all objects
		configpolicies := &configpolicyv1.ConfigurationPolicyList{}
		if err := c.hostingCache.List(ctx, configpolicies); err != nil {
			return err
		}

		for _, configpolicy := range configpolicies.Items {
			controllerContext.Queue().Add(fmt.Sprintf("%s/%s", configpolicy.Namespace, configpolicy.Name))
		}
		return nil
	}

	namespace, name, err := kubecache.SplitMetaNamespaceKey(queueKey)
	utilruntime.HandleError(err)

	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	_, err = c.configurationPolicyReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
	return err
}

type hostingPolicyController struct {
	hostingPolicyReconciler *statussync.PolicyReconciler
	templateReconciler      *templatesync.PolicyReconciler
	hostingCache            cache.Cache
}

func newHostingPolicyController(ctx context.Context,
	hostingPolicyReconciler *statussync.PolicyReconciler,
	templateReconciler *templatesync.PolicyReconciler,
	hostingCache cache.Cache,
	src source.Source) factory.Controller {
	controllerName := "hostingpolicy-controller"
	recorder := util.NewLoggingRecorder(controllerName)
	syncCtx := factory.NewSyncContext(controllerName, recorder)

	controller := &hostingPolicyController{
		hostingPolicyReconciler: hostingPolicyReconciler,
		templateReconciler:      templateReconciler,
		hostingCache:            hostingCache,
	}

	policyInformer, err := hostingCache.GetInformer(ctx, &policyv1.Policy{})
	utilruntime.Must(err)

	eventInformer, err := hostingCache.GetInformer(ctx, &corev1.Event{})
	utilruntime.Must(err)

	utilruntime.Must(src.Start(ctx, &handler.EnqueueRequestForObject{}, syncCtx.Queue()))

	if _, err := eventInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			event, ok := obj.(*corev1.Event)
			if !ok {
				return
			}
			if event.InvolvedObject.Kind != policyv1.Kind ||
				event.InvolvedObject.APIVersion != policiesv1APIVersion {
				return
			}
			syncCtx.Queue().Add(fmt.Sprintf("%s/%s", event.InvolvedObject.Namespace, event.InvolvedObject.Name))
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			event, ok := newObj.(*corev1.Event)
			if !ok {
				return
			}

			if event.InvolvedObject.Kind != policyv1.Kind ||
				event.InvolvedObject.APIVersion != policiesv1APIVersion {
				return
			}

			syncCtx.Queue().Add(fmt.Sprintf("%s/%s", event.InvolvedObject.Namespace, event.InvolvedObject.Name))
		},
		DeleteFunc: func(obj interface{}) {
			//do nothing
		},
	}); err != nil {
		utilruntime.HandleError(err)
	}

	return factory.New().WithSyncContext(syncCtx).
		WithSync(controller.sync).
		WithInformersQueueKeyFunc(func(obj runtime.Object) string {
			key, _ := kubecache.MetaNamespaceKeyFunc(obj)
			return key
		}, policyInformer).
		WithBareInformers(eventInformer).
		ToController("HostingPolicyController", recorder)
}

func (c *hostingPolicyController) sync(ctx context.Context, controllerContext factory.SyncContext) error {
	queueKey := controllerContext.QueueKey()
	if queueKey == factory.DefaultQueueKey {
		// triggered by resync, requeue all objects
		policies := &policyv1.PolicyList{}
		if err := c.hostingCache.List(ctx, policies); err != nil {
			return err
		}

		for _, policy := range policies.Items {
			controllerContext.Queue().Add(fmt.Sprintf("%s/%s", policy.Namespace, policy.Name))
		}
		return nil
	}

	namespace, name, err := kubecache.SplitMetaNamespaceKey(queueKey)
	utilruntime.HandleError(err)

	namespacedName := types.NamespacedName{Namespace: namespace, Name: name}
	if _, err := c.hostingPolicyReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName}); err != nil {
		return err
	}

	_, err = c.templateReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: namespacedName})
	return err
}
