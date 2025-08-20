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
	"github.com/NVIDIA/grove/operator/internal/webhook/admission/pgs/defaulting"
	"github.com/NVIDIA/grove/operator/internal/webhook/admission/pgs/validation"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// WebhookRegister interface defines webhook handlers that can be registered with the manager
type WebhookRegister interface {
	RegisterWithManager(mgr manager.Manager) error
}

// TestEnv represents a built and started test environment
type TestEnv struct {
	T       *testing.T
	Client  client.Client
	Manager manager.Manager
	Ctx     context.Context
	Objects []client.Object

	// Internal fields for lifecycle management
	env              *envtest.Environment
	cancel           context.CancelFunc
	started          bool
	namespaceConfigs map[string]*corev1.Namespace
	controllers      map[ControllerType]bool
	webhooks         map[WebhookType]bool
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
		_ = te.Client.Delete(te.Ctx, ns)
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

	// Create client for setup
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
	err = te.createManager(cfg)
	if err != nil {
		return err
	}

	if te.requiredControllers() > 0 {
		if err = te.registerControllers(defaultTestOperatorConfig()); err != nil {
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
	// only after the manager is started
	// to ensure cache is synced
	// and resources are available
	te.Client = te.Manager.GetClient()

	te.started = true
	return nil
}

func (te *TestEnv) requiredControllers() int {
	return len(te.controllers)
}

func (te *TestEnv) requiredWebhooks() bool {
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
func (te *TestEnv) createManager(cfg *rest.Config) error {
	// Add webhook server configuration if webhooks are enabled
	managerOpts := ctrl.Options{
		Scheme:                 te.env.Scheme,
		LeaderElection:         false, // Disable leader election in tests
		HealthProbeBindAddress: "0",
	}
	if te.requiredWebhooks() {
		webhookServer := webhook.NewServer(webhook.Options{
			Port:    te.env.WebhookInstallOptions.LocalServingPort,
			Host:    te.env.WebhookInstallOptions.LocalServingHost,
			CertDir: te.env.WebhookInstallOptions.LocalServingCertDir,
		})
		te.T.Logf("Initialized webhook server with options: %+v", webhookServer)
		managerOpts.WebhookServer = webhookServer
	}

	mgr, err := ctrl.NewManager(cfg, managerOpts)
	if err != nil {
		return err
	}
	te.Manager = mgr
	return nil
}

// registerControllers registers enabled controllers with the manager
func (te *TestEnv) registerControllers(operatorCfg *configv1alpha1.OperatorConfiguration) error {
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
	for webhookType := range te.webhooks {
		switch webhookType {
		case WebhookValidation:
			if err := te.registerValidationWebhook(); err != nil {
				return err
			}
		case WebhookMutation:
			if err := te.registerMutationWebhook(); err != nil {
				return err
			}
		default:
			te.T.Logf("Unknown webhook requested: %s", webhookType)
		}
	}
	return nil
}

// registerValidationWebhook registers validation webhooks with the manager
func (te *TestEnv) registerValidationWebhook() error {
	validatingWebhook := validation.NewHandler(te.Manager)
	if err := validatingWebhook.RegisterWithManager(te.Manager); err != nil {
		return fmt.Errorf("failed to register validation webhook: %w", err)
	}
	// For now, just log that it would be registered
	te.T.Logf("Registered validation webhook")
	return nil
}

// registerMutationWebhook registers mutation webhooks with the manager
func (te *TestEnv) registerMutationWebhook() error {
	defaultingWebhook := defaulting.NewHandler(te.Manager)
	if err := defaultingWebhook.RegisterWithManager(te.Manager); err != nil {
		return fmt.Errorf("failed to register mutation webhook: %w", err)
	}
	te.T.Logf("Registered mutation webhook")
	return nil
}
