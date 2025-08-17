package framework

import (
	"context"
	"testing"

	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	groveclient "github.com/NVIDIA/grove/operator/internal/client"
	grovecontroller "github.com/NVIDIA/grove/operator/internal/controller"
	grovelogger "github.com/NVIDIA/grove/operator/internal/logger"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// EnvBuilder builds a test environment with a fluent API
type EnvBuilder struct {
	t *testing.T

	// Core components
	env     *envtest.Environment
	manager ctrl.Manager
	client  client.Client
	ctx     context.Context
	cancel  context.CancelFunc

	// Configuration
	crdPaths             []string
	additionalCRDs       []string
	scheme               *runtime.Scheme
	installGroveCRDs     bool
	installSchedulerCRDs bool

	// Controllers
	controllers    map[string]bool
	allControllers bool
	webhooks       bool

	// RBAC
	installRBAC    bool
	customRBAC     []rbacv1.PolicyRule
	serviceAccount string

	// Namespaces
	namespaces map[string]*corev1.Namespace

	// Pre-created objects
	objects []client.Object

	// Manager options
	managerOpts []ManagerOption

	// Error tracking
	err error
}

// NewEnvBuilder creates a new environment builder
func NewEnvBuilder(t *testing.T) *EnvBuilder {
	return &EnvBuilder{
		t:           t,
		crdPaths:    []string{},
		controllers: make(map[string]bool),
		namespaces:  make(map[string]*corev1.Namespace),
		objects:     []client.Object{},
		scheme:      groveclient.Scheme, // Use Grove's production scheme
	}
}

// WithGroveCRDs adds Grove operator CRDs
func (b *EnvBuilder) WithGroveCRDs() *EnvBuilder {
	b.installGroveCRDs = true
	return b
}

// WithSchedulerCRDs adds scheduler CRDs (for PodGang)
func (b *EnvBuilder) WithSchedulerCRDs() *EnvBuilder {
	b.installSchedulerCRDs = true
	return b
}

// WithCRDPath adds a custom CRD directory path
func (b *EnvBuilder) WithCRDPath(path string) *EnvBuilder {
	b.crdPaths = append(b.crdPaths, path)
	return b
}

// WithCRDFiles adds specific CRD files
func (b *EnvBuilder) WithCRDFiles(files ...string) *EnvBuilder {
	b.additionalCRDs = append(b.additionalCRDs, files...)
	return b
}

// WithScheme sets a custom scheme
func (b *EnvBuilder) WithScheme(scheme *runtime.Scheme) *EnvBuilder {
	b.scheme = scheme
	return b
}

// WithAllControllers enables all controllers
func (b *EnvBuilder) WithAllControllers() *EnvBuilder {
	b.allControllers = true
	return b
}

// WithController enables a specific controller
func (b *EnvBuilder) WithController(name string) *EnvBuilder {
	b.controllers[name] = true
	return b
}

// WithPodGangSetController enables the PodGangSet controller
func (b *EnvBuilder) WithPodGangSetController() *EnvBuilder {
	return b.WithController("podgangset")
}

// WithPodCliqueController enables the PodClique controller
func (b *EnvBuilder) WithPodCliqueController() *EnvBuilder {
	return b.WithController("podclique")
}

// WithScalingGroupController enables the PodCliqueScalingGroup controller
func (b *EnvBuilder) WithScalingGroupController() *EnvBuilder {
	return b.WithController("scalinggroup")
}

// WithControllers enables multiple controllers
func (b *EnvBuilder) WithControllers(names ...string) *EnvBuilder {
	for _, name := range names {
		b.controllers[name] = true
	}
	return b
}

// WithWebhooks enables webhook server
func (b *EnvBuilder) WithWebhooks() *EnvBuilder {
	b.webhooks = true
	return b
}

// WithConcurrentSyncs sets concurrent syncs for all controllers
func (b *EnvBuilder) WithConcurrentSyncs(syncs int32) *EnvBuilder {
	b.managerOpts = append(b.managerOpts, WithConcurrentSyncs(syncs))
	return b
}

// WithRBAC installs default RBAC resources
func (b *EnvBuilder) WithRBAC() *EnvBuilder {
	b.installRBAC = true
	b.serviceAccount = "grove-operator-test"
	return b
}

// WithServiceAccount sets a custom service account name
func (b *EnvBuilder) WithServiceAccount(name string) *EnvBuilder {
	b.serviceAccount = name
	b.installRBAC = true
	return b
}

// WithCustomRBAC adds custom RBAC rules
func (b *EnvBuilder) WithCustomRBAC(rules ...rbacv1.PolicyRule) *EnvBuilder {
	b.customRBAC = append(b.customRBAC, rules...)
	b.installRBAC = true
	return b
}

// WithReadOnlyRBAC installs read-only RBAC
func (b *EnvBuilder) WithReadOnlyRBAC() *EnvBuilder {
	b.installRBAC = true
	b.customRBAC = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"grove.io", "scheduler.grove.io", ""},
			Resources: []string{"*"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}
	return b
}

// WithNamespace creates a namespace
func (b *EnvBuilder) WithNamespace(name string) *EnvBuilder {
	b.namespaces[name] = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}
	return b
}

// WithNamespaces creates multiple namespaces
func (b *EnvBuilder) WithNamespaces(names ...string) *EnvBuilder {
	for _, name := range names {
		b.WithNamespace(name)
	}
	return b
}

// WithLabeledNamespace creates a namespace with labels
func (b *EnvBuilder) WithLabeledNamespace(name string, labels map[string]string) *EnvBuilder {
	b.namespaces[name] = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
	return b
}

// WithTestNamespace creates a namespace with a generated name
func (b *EnvBuilder) WithTestNamespace() *EnvBuilder {
	return b
}

// WithObject adds an object to be created after environment setup
func (b *EnvBuilder) WithObject(obj client.Object) *EnvBuilder {
	b.objects = append(b.objects, obj)
	return b
}

// WithObjects adds multiple objects to be created
func (b *EnvBuilder) WithObjects(objs ...client.Object) *EnvBuilder {
	b.objects = append(b.objects, objs...)
	return b
}

// WithConfigMap adds a ConfigMap to be created
func (b *EnvBuilder) WithConfigMap(name, namespace string, data map[string]string) *EnvBuilder {
	return b
}

// WithSecret adds a Secret to be created
func (b *EnvBuilder) WithSecret(name, namespace string, data map[string][]byte) *EnvBuilder {
	return b
}

// Build creates and starts the test environment
func (b *EnvBuilder) Build() *TestEnv {
	if b.err != nil {
		b.t.Fatalf("builder error: %v", b.err)
	}

	// Initialize controller-runtime logger for debugging
	ctrl.SetLogger(grovelogger.MustNewLogger(true, configv1alpha1.DebugLevel, configv1alpha1.LogFormatJSON))
	b.t.Logf("Initialized controller-runtime logger with debug level")

	// Create context
	b.ctx, b.cancel = context.WithCancel(context.Background())

	// Setup envtest environment
	b.env = &envtest.Environment{
		CRDDirectoryPaths:     b.crdPaths,
		ErrorIfCRDPathMissing: len(b.crdPaths) > 0,
	}

	// Start environment
	cfg, err := b.env.Start()
	if err != nil {
		b.t.Fatalf("failed to start envtest: %v", err)
	}

	// Create client for CRD installation
	b.client, err = client.New(cfg, client.Options{Scheme: b.scheme})
	if err != nil {
		b.t.Fatalf("failed to create client: %v", err)
	}

	// Install Grove CRDs if requested
	if b.installGroveCRDs {
		if err := installGroveCRDs(b.ctx, b.client); err != nil {
			b.t.Fatalf("failed to install Grove CRDs: %v", err)
		}
	}

	// Install Scheduler CRDs if requested
	if b.installSchedulerCRDs {
		if err := installSchedulerCRDs(b.ctx, b.client); err != nil {
			b.t.Fatalf("failed to install Scheduler CRDs: %v", err)
		}
	}

	// Create namespaces
	for _, ns := range b.namespaces {
		if err := b.client.Create(b.ctx, ns); err != nil {
			b.t.Fatalf("failed to create namespace %s: %v", ns.Name, err)
		}
	}

	// Create manager if controllers are needed
	if b.needsControllers() {
		if err := b.createManager(cfg); err != nil {
			b.t.Fatalf("failed to create manager: %v", err)
		}

		// Register controllers
		if err := b.registerControllers(); err != nil {
			b.t.Fatalf("failed to register controllers: %v", err)
		}
		b.t.Logf("Successfully registered controllers")

		// Start manager
		go func() {
			b.t.Logf("Starting controller manager...")
			if err := b.manager.Start(b.ctx); err != nil {
				b.t.Logf("manager stopped: %v", err)
			}
		}()

		// Wait for cache sync
		b.t.Logf("Waiting for cache sync...")
		if !b.manager.GetCache().WaitForCacheSync(b.ctx) {
			b.t.Fatal("cache sync timeout")
		}
		b.t.Logf("Cache sync completed successfully")

		// Update client to use manager's client
		b.client = b.manager.GetClient()
		b.t.Logf("Updated client to use manager's client")
	}

	// Register cleanup
	b.t.Cleanup(func() {
		b.cleanup()
	})

	return &TestEnv{
		T:          b.t,
		Client:     b.client,
		Manager:    b.manager,
		Ctx:        b.ctx,
		Namespaces: b.getNamespaceNames(),
		Objects:    b.objects,
	}
}

// needsControllers returns true if any controllers are enabled
func (b *EnvBuilder) needsControllers() bool {
	return b.allControllers || len(b.controllers) > 0
}

// createManager creates the controller manager
func (b *EnvBuilder) createManager(cfg *rest.Config) error {
	testMgr, err := CreateTestManager(cfg, b.managerOpts...)
	if err != nil {
		return err
	}

	b.manager = testMgr.Manager
	return nil
}

// registerControllers registers enabled controllers
func (b *EnvBuilder) registerControllers() error {
	operatorCfg := b.buildOperatorConfig()
	return grovecontroller.RegisterControllers(b.manager, operatorCfg.Controllers)
}

// buildOperatorConfig builds the operator configuration
func (b *EnvBuilder) buildOperatorConfig() *configv1alpha1.OperatorConfiguration {
	cfg := defaultTestOperatorConfig()

	// Apply controller configuration
	if !b.allControllers && len(b.controllers) > 0 {
		// Disable all controllers by default
		cfg.Controllers.PodGangSet.ConcurrentSyncs = ptr.To(0)
		cfg.Controllers.PodClique.ConcurrentSyncs = ptr.To(0)
		cfg.Controllers.PodCliqueScalingGroup.ConcurrentSyncs = ptr.To(0)

		// Enable specific controllers
		for name := range b.controllers {
			switch name {
			case "podgangset":
				cfg.Controllers.PodGangSet.ConcurrentSyncs = ptr.To(1)
				b.t.Logf("Enabled PodGangSet controller with 1 concurrent sync")
			case "podclique":
				cfg.Controllers.PodClique.ConcurrentSyncs = ptr.To(1)
				b.t.Logf("Enabled PodClique controller with 1 concurrent sync")
			case "scalinggroup":
				cfg.Controllers.PodCliqueScalingGroup.ConcurrentSyncs = ptr.To(1)
				b.t.Logf("Enabled PodCliqueScalingGroup controller with 1 concurrent sync")
			default:
				b.t.Logf("Unknown controller requested: %s", name)
			}
		}
	} else if b.allControllers {
		b.t.Logf("Enabled all controllers")
	}

	return cfg
}

// cleanup cleans up the test environment
func (b *EnvBuilder) cleanup() {
	if b.cancel != nil {
		b.cancel()
	}
	if b.env != nil {
		_ = b.env.Stop()
	}
}

// getNamespaceNames returns the names of created namespaces
func (b *EnvBuilder) getNamespaceNames() []string {
	names := make([]string, 0, len(b.namespaces))
	for name := range b.namespaces {
		names = append(names, name)
	}
	return names
}

// WithBasicSetup adds common setup for most tests
func (b *EnvBuilder) WithBasicSetup() *EnvBuilder {
	return b.
		WithGroveCRDs().
		WithSchedulerCRDs().
		WithAllControllers().
		WithRBAC().
		WithTestNamespace()
}

// WithControllerTest sets up for controller testing
func (b *EnvBuilder) WithControllerTest(controllerName string) *EnvBuilder {
	return b.
		WithGroveCRDs().
		WithSchedulerCRDs().
		WithController(controllerName).
		WithRBAC().
		WithTestNamespace()
}

// WithWebhookTest sets up for webhook testing
func (b *EnvBuilder) WithWebhookTest() *EnvBuilder {
	return b.
		WithGroveCRDs().
		WithWebhooks().
		WithRBAC().
		WithTestNamespace()
}

// WithAPITest sets up for API-only testing (no controllers/webhooks)
func (b *EnvBuilder) WithAPITest() *EnvBuilder {
	return b.
		WithGroveCRDs().
		WithTestNamespace()
}
