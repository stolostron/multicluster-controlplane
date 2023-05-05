package addons

import (
	"context"
	"os"

	depclient "github.com/stolostron/kubernetes-dependency-watches/client"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	"open-cluster-management.io/config-policy-controller/controllers"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/secretsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/specsync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/statussync"
	"open-cluster-management.io/governance-policy-framework-addon/controllers/templatesync"
	policiesv1 "open-cluster-management.io/governance-policy-propagator/api/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type PolicyAgentConfig struct {
	DecryptionConcurrency uint8
	EvaluationConcurrency uint8
	EnableMetrics         bool
	Frequency             uint
}

func StartPolicyAgent(
	ctx context.Context,
	clusterName string,
	kubeConfig *rest.Config,
	hubManager, hostingManager ctrl.Manager,
	config *PolicyAgentConfig) error {
	instanceName, _ := os.Hostname() // on an error, instanceName will be empty, which is ok

	targetK8sClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}

	targetK8sDynamicClient, err := dynamic.NewForConfig(kubeConfig)
	if err != nil {
		return err
	}
	// create target namespace if it doesn't exist
	err = hostingManager.GetClient().Create(ctx, &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterName,
		},
	}, &client.CreateOptions{})
	if err != nil {
		if !k8serrors.IsAlreadyExists(err) {
			return err
		}
	}

	reconciler := controllers.ConfigurationPolicyReconciler{
		Client:                 hostingManager.GetClient(),
		DecryptionConcurrency:  config.DecryptionConcurrency,
		EvaluationConcurrency:  config.EvaluationConcurrency,
		Scheme:                 hostingManager.GetScheme(),
		Recorder:               hostingManager.GetEventRecorderFor(controllers.ControllerName),
		InstanceName:           instanceName,
		TargetK8sClient:        targetK8sClient,
		TargetK8sDynamicClient: targetK8sDynamicClient,
		TargetK8sConfig:        kubeConfig,
		EnableMetrics:          config.EnableMetrics,
	}

	if err := reconciler.SetupWithManager(hostingManager); err != nil {
		return err
	}

	go func() {
		reconciler.PeriodicallyExecConfigPolicies(ctx, config.Frequency, hostingManager.Elected())
	}()

	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(
		&corev1.EventSinkImpl{Interface: targetK8sClient.CoreV1().Events(clusterName)},
	)

	eventsScheme := runtime.NewScheme()
	utilruntime.Must(v1.AddToScheme(eventsScheme))
	utilruntime.Must(policiesv1.AddToScheme(eventsScheme))

	managedRecorder := eventBroadcaster.NewRecorder(eventsScheme, v1.EventSource{Component: specsync.ControllerName})

	if err := (&specsync.PolicyReconciler{
		HubClient:       hubManager.GetClient(),
		ManagedClient:   hostingManager.GetClient(),
		ManagedRecorder: managedRecorder,
		Scheme:          hubManager.GetScheme(),
		TargetNamespace: clusterName,
	}).SetupWithManager(hubManager); err != nil {
		return err
	}

	if err := (&secretsync.SecretReconciler{
		Client:          hubManager.GetClient(),
		ManagedClient:   hostingManager.GetClient(),
		Scheme:          hubManager.GetScheme(),
		TargetNamespace: clusterName,
	}).SetupWithManager(hubManager); err != nil {
		return err
	}

	hubKubeClient := kubernetes.NewForConfigOrDie(hubManager.GetConfig())

	hubEventBroadcaster := record.NewBroadcaster()

	hubEventBroadcaster.StartRecordingToSink(
		&corev1.EventSinkImpl{Interface: hubKubeClient.CoreV1().Events(clusterName)},
	)

	hubRecorder := hubEventBroadcaster.NewRecorder(eventsScheme, v1.EventSource{Component: statussync.ControllerName})

	if err := (&statussync.PolicyReconciler{
		ClusterNamespaceOnHub: clusterName,
		HubClient:             hubManager.GetClient(),
		HubRecorder:           hubRecorder,
		ManagedClient:         hostingManager.GetClient(),
		ManagedRecorder:       hostingManager.GetEventRecorderFor(statussync.ControllerName),
		Scheme:                hostingManager.GetScheme(),
	}).SetupWithManager(hostingManager); err != nil {
		return err
	}

	depReconciler, depEvents := depclient.NewControllerRuntimeSource()

	watcher, err := depclient.New(kubeConfig, depReconciler, nil)
	if err != nil {
		return err
	}

	templateReconciler := &templatesync.PolicyReconciler{
		Client:           hostingManager.GetClient(),
		DynamicWatcher:   watcher,
		Scheme:           hostingManager.GetScheme(),
		Config:           hostingManager.GetConfig(),
		Recorder:         hostingManager.GetEventRecorderFor(templatesync.ControllerName),
		ClusterNamespace: clusterName,
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
