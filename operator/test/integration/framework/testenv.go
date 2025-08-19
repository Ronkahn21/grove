package framework

import (
	"context"
	"fmt"
	"testing"

	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/controller/podclique"
	"github.com/NVIDIA/grove/operator/internal/controller/podcliquescalinggroup"
	"github.com/NVIDIA/grove/operator/internal/controller/podgangset"
	grovelogger "github.com/NVIDIA/grove/operator/internal/logger"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type WebhookRegister interface {
	RegisterWithManager(mgr manager.Manager) error
}

type WebhookRegisterFn func(mgr manager.Manager) WebhookRegister

// TestEnv represents a built and started test environment
type TestEnv struct {
	T          *testing.T
	Client     client.Client
	Manager    manager.Manager
	Ctx        context.Context
	Namespaces []string
	Objects    []client.Object

	// Internal fields for lifecycle management
	env              *envtest.Environment
	cancel           context.CancelFunc
	started          bool
	namespaceConfigs map[string]*corev1.Namespace
	controllers      map[ControllerType]bool
	webhooks         []WebhookRegisterFn
	managerOpts      []ManagerOption
}

// CreateNamespace creates an additional namespace
func (te *TestEnv) CreateNamespace(name string) (string, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := te.Client.Create(te.Ctx, ns); err != nil {
		return "", fmt.Errorf("failed to create namespace: %w", err)
	}

	te.T.Cleanup(func() {
		_ = te.Client.Delete(context.Background(), ns)
	})

	return name, nil
}

// Start starts the test environment and all its components
func (te *TestEnv) Start() error {
	if te.started {
		return nil // Already started
	}

	te.initializeLogger()

	// Start control plane
	cfg, err := te.startEnvironment()
	if err != nil {
		return err
	}

	// Create client for CRD installation and setup
	kubeClient, err := te.createClient()
	if err != nil {
		return err
	}
	te.Client = kubeClient

	// Create namespaces
	if err = te.createNamespaces(); err != nil {
		return err
	}
	// Create and start manager if controllers or webhooks are needed
	mgr, err := te.createManager(cfg)
	if err != nil {
		return err
	}
	te.Manager = mgr
	if len(te.controllers) > 0 {
		if err = te.registerControllers(); err != nil {
			return err
		}
	}
	if te.requiredWebhooks() {
		if err = te.registerWebhooks(); err != nil {
			return err
		}
	}
	if err = te.startManager(); err != nil {
		return err
	}
	// Update client to use manager's client
	te.Client = te.Manager.GetClient()

	te.started = true
	return nil
}

func (te *TestEnv) requiredWebhooks() bool {
	// Check if any webhooks are enabled
	return len(te.webhooks) > 0
}

// Shutdown stops the test environment and cleans up resources
func (te *TestEnv) Shutdown() {
	if !te.started {
		return // already stopped or never started
	}

	if te.cancel != nil {
		te.cancel()
	}

	if te.env != nil {
		_ = te.env.Stop()
	}

	te.started = false
}

// initializeLogger sets up the controllers-runtime logger
func (te *TestEnv) initializeLogger() {
	ctrl.SetLogger(grovelogger.MustNewLogger(true, configv1alpha1.DebugLevel, configv1alpha1.LogFormatJSON))
	te.T.Logf("Initialized controllers-runtime logger with debug level")
}

// startEnvironment starts the envtest control plane
func (te *TestEnv) startEnvironment() (*rest.Config, error) {
	cfg, err := te.env.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start envtest: %w", err)
	}
	return cfg, nil
}

// createClient creates a Kubernetes client
func (te *TestEnv) createClient() (client.Client, error) {
	kubeClient, err := client.New(te.env.Config, client.Options{Scheme: te.env.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	return kubeClient, nil
}

// createNamespaces creates the configured namespaces
func (te *TestEnv) createNamespaces() error {
	for _, ns := range te.namespaceConfigs {
		if err := te.Client.Create(te.Ctx, ns); err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", ns.Name, err)
		}
	}
	return nil
}

// createManager creates the controllers manager
func (te *TestEnv) createManager(cfg *rest.Config) (manager.Manager, error) {
	opts := te.managerOpts

	// Add webhook server configuration if webhooks are enabled
	if te.requiredWebhooks() {
		webhookServer := webhook.NewServer(webhook.Options{
			Host:    te.env.WebhookInstallOptions.LocalServingHost,
			Port:    te.env.WebhookInstallOptions.LocalServingPort,
			CertDir: te.env.WebhookInstallOptions.LocalServingCertDir,
		})
		opts = append(opts, WithWebhookServer(webhookServer))
		te.T.Logf("Created webhook server: Host=%s, Port=%d, CertDir=%s",
			te.env.WebhookInstallOptions.LocalServingHost,
			te.env.WebhookInstallOptions.LocalServingPort,
			te.env.WebhookInstallOptions.LocalServingCertDir)
	}

	testMgr, err := CreateTestManager(cfg, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create manager: %w", err)
	}
	return testMgr.Manager, nil
}

// registerControllers registers enabled controllers with the manager
func (te *TestEnv) registerControllers() error {
	operatorCfsg := &configv1alpha1.OperatorConfiguration{
		Controllers: configv1alpha1.ControllerConfiguration{
			PodGangSet: configv1alpha1.PodGangSetControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
			PodClique: configv1alpha1.PodCliqueControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
			PodCliqueScalingGroup: configv1alpha1.PodCliqueScalingGroupControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
		},
	}
	return te.registerSpecificControllers(operatorCfsg)
}

// registerSpecificControllers registers only the enabled controllers
func (te *TestEnv) registerSpecificControllers(operatorCfg *configv1alpha1.OperatorConfiguration) error {
	for controllerType := range te.controllers {
		switch controllerType {
		case ControllerPodGangSet:
			if err := te.registerPodGangSetController(operatorCfg.Controllers.PodGangSet); err != nil {
				return err
			}
		case ControllerPodClique:
			if err := te.registerPodCliqueController(operatorCfg.Controllers.PodClique); err != nil {
				return err
			}
		case ControllerScalingGroup:
			if err := te.registerScalingGroupController(operatorCfg.Controllers.PodCliqueScalingGroup); err != nil {
				return err
			}
		default:
			te.T.Logf("Unknown controllers requested: %s", controllerType)
		}
	}
	return nil
}

// registerPodGangSetController registers the PodGangSet controllers
func (te *TestEnv) registerPodGangSetController(config configv1alpha1.PodGangSetControllerConfiguration) error {
	reconciler := podgangset.NewReconciler(te.Manager, config)
	if err := reconciler.RegisterWithManager(te.Manager); err != nil {
		return fmt.Errorf("failed to register PodGangSet controllers: %w", err)
	}
	te.T.Logf("Registered PodGangSet controllers")
	return nil
}

// registerPodCliqueController registers the PodClique controllers
func (te *TestEnv) registerPodCliqueController(config configv1alpha1.PodCliqueControllerConfiguration) error {
	reconciler := podclique.NewReconciler(te.Manager, config)
	if err := reconciler.RegisterWithManager(te.Manager); err != nil {
		return fmt.Errorf("failed to register PodClique controllers: %w", err)
	}
	te.T.Logf("Registered PodClique controllers")
	return nil
}

// registerScalingGroupController registers the PodCliqueScalingGroup controllers
func (te *TestEnv) registerScalingGroupController(config configv1alpha1.PodCliqueScalingGroupControllerConfiguration) error {
	reconciler := podcliquescalinggroup.NewReconciler(te.Manager, config)
	if err := reconciler.RegisterWithManager(te.Manager); err != nil {
		return fmt.Errorf("failed to register PodCliqueScalingGroup controllers: %w", err)
	}
	te.T.Logf("Registered PodCliqueScalingGroup controllers")
	return nil
}

// startManager starts the manager and waits for cache sync
func (te *TestEnv) startManager() error {
	go func() {
		te.T.Logf("Starting controllers manager...")
		if err := te.Manager.Start(te.Ctx); err != nil {
			te.T.Logf("manager stopped: %v", err)
		}
	}()

	te.T.Logf("Waiting for cache sync...")
	if !te.Manager.GetCache().WaitForCacheSync(te.Ctx) {
		return fmt.Errorf("cache sync timeout")
	}
	te.T.Logf("Cache sync completed successfully")
	return nil
}

// registerWebhooks registers webhooks with the manager
func (te *TestEnv) registerWebhooks() error {
	for _, webhookFn := range te.webhooks {
		webhookHandler := webhookFn(te.Manager)
		if err := webhookHandler.RegisterWithManager(te.Manager); err != nil {
			return fmt.Errorf("failed to register webhook: %w", err)
		}
		te.T.Logf("Registered webhook: %T", webhookHandler)
	}
	return nil
}
