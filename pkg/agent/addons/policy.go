package addons

import (
	"context"
	"os"

	depclient "github.com/stolostron/kubernetes-dependency-watches/client"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"

	"open-cluster-management.io/config-policy-controller/controllers"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/specsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/statussync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/templatesync"

	ctrl "sigs.k8s.io/controller-runtime"
)

type PolicyAgentConfig struct {
	DecryptionConcurrency uint8
	EvaluationConcurrency uint8
	EnableMetrics         bool
	Frequency             uint
}

func StartPolicyAgent(
	ctx context.Context,
	scheme *runtime.Scheme,
	clusterName string,
	hubKubeConfig, hostingKubeConfig, spokeKubeConfig *rest.Config,
	hubManager, hostingManager ctrl.Manager,
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

	spokeClient, err := kubernetes.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	spokeDynamicClient, err := dynamic.NewForConfig(spokeKubeConfig)
	if err != nil {
		return err
	}

	hubEventBroadcaster := record.NewBroadcaster()
	hubEventBroadcaster.StartRecordingToSink(
		&clientcorev1.EventSinkImpl{Interface: hubKubeClient.CoreV1().Events(clusterName)},
	)

	spokeEventBroadcaster := record.NewBroadcaster()
	spokeEventBroadcaster.StartRecordingToSink(
		&clientcorev1.EventSinkImpl{Interface: spokeClient.CoreV1().Events(clusterName)},
	)

	// create target namespace if it doesn't exist
	targetNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterName}}
	if _, err := hostingKubeClient.CoreV1().Namespaces().Create(
		ctx, targetNamespace, metav1.CreateOptions{}); err != nil && !k8serrors.IsAlreadyExists(err) {
		return err
	}

	reconciler := controllers.ConfigurationPolicyReconciler{
		Client:                 hostingManager.GetClient(),
		DecryptionConcurrency:  config.DecryptionConcurrency,
		EvaluationConcurrency:  config.EvaluationConcurrency,
		Scheme:                 hostingManager.GetScheme(),
		Recorder:               hostingManager.GetEventRecorderFor(controllers.ControllerName),
		InstanceName:           instanceName,
		TargetK8sClient:        spokeClient,
		TargetK8sDynamicClient: spokeDynamicClient,
		TargetK8sConfig:        spokeKubeConfig,
		EnableMetrics:          config.EnableMetrics,
	}

	if err := reconciler.SetupWithManager(hostingManager); err != nil {
		return err
	}

	go func() {
		reconciler.PeriodicallyExecConfigPolicies(ctx, config.Frequency, hostingManager.Elected())
	}()

	if err := (&specsync.PolicyReconciler{
		HubClient:       hubManager.GetClient(),
		ManagedClient:   hostingManager.GetClient(),
		ManagedRecorder: spokeEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: specsync.ControllerName}),
		Scheme:          scheme,
		TargetNamespace: clusterName,
	}).SetupWithManager(hubManager); err != nil {
		return err
	}

	if err := (&secretsync.SecretReconciler{
		Client:          hubManager.GetClient(),
		ManagedClient:   hostingManager.GetClient(),
		Scheme:          scheme,
		TargetNamespace: clusterName,
	}).SetupWithManager(hubManager); err != nil {
		return err
	}

	if err := (&statussync.PolicyReconciler{
		ClusterNamespaceOnHub: clusterName,
		HubClient:             hubManager.GetClient(),
		HubRecorder:           hubEventBroadcaster.NewRecorder(scheme, corev1.EventSource{Component: statussync.ControllerName}),
		ManagedClient:         hostingManager.GetClient(),
		ManagedRecorder:       hostingManager.GetEventRecorderFor(statussync.ControllerName),
		Scheme:                scheme,
	}).SetupWithManager(hostingManager); err != nil {
		return err
	}

	depReconciler, depEvents := depclient.NewControllerRuntimeSource()

	watcher, err := depclient.New(hostingManager.GetConfig(), depReconciler, nil)
	if err != nil {
		return err
	}

	templateReconciler := &templatesync.PolicyReconciler{
		Client:           hostingManager.GetClient(),
		DynamicWatcher:   watcher,
		Scheme:           scheme,
		Config:           hostingManager.GetConfig(),
		Recorder:         hostingManager.GetEventRecorderFor(templatesync.ControllerName),
		ClusterNamespace: clusterName,
		Clientset:        kubernetes.NewForConfigOrDie(hostingManager.GetConfig()),
		InstanceName:     instanceName,
	}
	go func() {
		err := watcher.Start(ctx)
		if err != nil {
			panic(err)
		}
	}()

	// Wait until the dynamic watcher has started.
	<-watcher.Started()

	klog.Info("starting policy template sync controller")
	if err := templateReconciler.Setup(hostingManager, depEvents); err != nil {
		return err
	}

	return nil
}
