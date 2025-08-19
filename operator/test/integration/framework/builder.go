package framework

import (
	"context"
	"testing"

	groveclient "github.com/NVIDIA/grove/operator/internal/client"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

const (
	operatorCRDsDir  = "../charts/crds"
	schedulerCRDsDir = "../../scheduler/chart/crds"
)

var defaultWebhookRules = []admissionregistrationv1.RuleWithOperations{{
	Operations: []admissionregistrationv1.OperationType{
		admissionregistrationv1.Create,
		admissionregistrationv1.Update,
	},
	Rule: admissionregistrationv1.Rule{
		APIGroups:   []string{"grove.io"},
		APIVersions: []string{"v1alpha1"},
		Resources:   []string{"podgangsets"},
	},
}}

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
	controllers       map[ControllerType]bool
	allControllers    bool
	webhooks          bool
	validationWebhook bool
	mutatingWebhook   bool

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

// NewEnvBuilder creates a new environment builder with sensible defaults
func NewEnvBuilder(t *testing.T) *EnvBuilder {
	return &EnvBuilder{
		t:           t,
		crdPaths:    []string{"../charts/crds", "../../scheduler/chart/crds"},
		controllers: make(map[ControllerType]bool),
		namespaces:  make(map[string]*corev1.Namespace),
		objects:     []client.Object{},
		scheme:      groveclient.Scheme, // Use Grove's production scheme
		// Enable common requirements by default
		installGroveCRDs:     true,
		installSchedulerCRDs: true,
		installRBAC:          true,
		serviceAccount:       "grove-operator-test",
	}
}

// WithoutGroveCRDs disables Grove operator CRDs installation
func (b *EnvBuilder) WithoutGroveCRDs() *EnvBuilder {
	b.installGroveCRDs = false
	return b
}

// WithoutSchedulerCRDs disables scheduler CRDs installation
func (b *EnvBuilder) WithoutSchedulerCRDs() *EnvBuilder {
	b.installSchedulerCRDs = false
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

// WithController enables a specific controllers
func (b *EnvBuilder) WithController(controllerType ControllerType) *EnvBuilder {
	b.controllers[controllerType] = true
	return b
}

// WithPodGangSetController enables the PodGangSet controllers
func (b *EnvBuilder) WithPodGangSetController() *EnvBuilder {
	return b.WithController(ControllerPodGangSet)
}

// WithPodCliqueController enables the PodClique controllers
func (b *EnvBuilder) WithPodCliqueController() *EnvBuilder {
	return b.WithController(ControllerPodClique)
}

// WithScalingGroupController enables the PodCliqueScalingGroup controllers
func (b *EnvBuilder) WithScalingGroupController() *EnvBuilder {
	return b.WithController(ControllerScalingGroup)
}

// WithControllers enables multiple controllers
func (b *EnvBuilder) WithControllers(controllerTypes ...ControllerType) *EnvBuilder {
	for _, controllerType := range controllerTypes {
		b.controllers[controllerType] = true
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

// WithoutRBAC disables RBAC installation
func (b *EnvBuilder) WithoutRBAC() *EnvBuilder {
	b.installRBAC = false
	b.customRBAC = nil
	b.serviceAccount = ""
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

// WithObjects adds multiple objects to be created
func (b *EnvBuilder) WithObjects(objs ...client.Object) *EnvBuilder {
	b.objects = append(b.objects, objs...)
	return b
}

// Build creates the test environment but does not start it
func (b *EnvBuilder) Build() *TestEnv {
	if b.err != nil {
		b.t.Fatalf("builder error: %v", b.err)
	}

	// Create context
	b.ctx, b.cancel = context.WithCancel(context.Background())

	// Setup envtest environment
	b.env = &envtest.Environment{
		CRDDirectoryPaths:     b.crdPaths,
		ErrorIfCRDPathMissing: len(b.crdPaths) > 0,
	}
	webhookOptions := envtest.WebhookInstallOptions{}
	if b.validationWebhook {
		webhookOptions.ValidatingWebhooks = append(webhookOptions.ValidatingWebhooks, b.getPGSWebhookValidationConfig())
	}
	if b.mutatingWebhook {
		webhookOptions.MutatingWebhooks = append(webhookOptions.MutatingWebhooks, b.getPGSWebhookMutationConfig())
	}
	if len(webhookOptions.ValidatingWebhooks) > 0 || len(webhookOptions.MutatingWebhooks) > 0 {
		b.env.WebhookInstallOptions = webhookOptions
	}

	// Register cleanup
	b.t.Cleanup(func() {
		b.cleanup()
	})

	return &TestEnv{
		T:          b.t,
		Client:     nil, // Will be created in Start()
		Manager:    nil, // Will be created in Start()
		Ctx:        b.ctx,
		Namespaces: b.getNamespaceNames(),
		Objects:    b.objects,

		// Internal fields for lifecycle management
		env:     b.env,
		cancel:  b.cancel,
		started: false,

		// Configuration needed for startup
		scheme:               b.scheme,
		installGroveCRDs:     b.installGroveCRDs,
		installSchedulerCRDs: b.installSchedulerCRDs,
		namespaceConfigs:     b.namespaces,
		controllers:          b.controllers,
		allControllers:       b.allControllers,
		webhooks:             b.webhooks,
		managerOpts:          b.managerOpts,
	}
}

func (b *EnvBuilder) getPGSWebhookMutationConfig() *admissionregistrationv1.MutatingWebhookConfiguration {
	webhookConfig := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pgs-mutating-webhook-test",
		},
		Webhooks: []admissionregistrationv1.MutatingWebhook{{
			Name: "pgs.mutating.webhooks.grove.io",
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name: "grove-operator-test-webhook",
					Path: ptr.To("/webhooks/default-podgangset"),
				},
			},
			Rules:          defaultWebhookRules,
			FailurePolicy:  ptr.To(admissionregistrationv1.Fail),
			SideEffects:    ptr.To(admissionregistrationv1.SideEffectClassNone),
			TimeoutSeconds: ptr.To[int32](10),
		}},
	}
	return webhookConfig
}

func (b *EnvBuilder) getPGSWebhookValidationConfig() *admissionregistrationv1.ValidatingWebhookConfiguration {
	webhookConfig := admissionregistrationv1.ValidatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pgs-validating-webhook-test",
		},
		Webhooks: []admissionregistrationv1.ValidatingWebhook{{
			Name: "pgs.validating.webhooks.grove.io",
			ClientConfig: admissionregistrationv1.WebhookClientConfig{
				Service: &admissionregistrationv1.ServiceReference{
					Name: "grove-operator-test-webhook",
					Path: ptr.To("/webhooks/validate-podgangset"),
				},
			},
			Rules:          defaultWebhookRules,
			FailurePolicy:  ptr.To(admissionregistrationv1.Fail),
			SideEffects:    ptr.To(admissionregistrationv1.SideEffectClassNone),
			TimeoutSeconds: ptr.To[int32](10),
		}},
	}
	return &webhookConfig
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
