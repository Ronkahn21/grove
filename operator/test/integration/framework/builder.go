package framework

import (
	"context"
	"testing"

	groveclient "github.com/NVIDIA/grove/operator/internal/client"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

// EnvBuilder builds a test environment with a fluent API
type EnvBuilder struct {
	t *testing.T

	// Core components
	env    *envtest.Environment
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration
	crds                 []*apiextensionsv1.CustomResourceDefinition
	scheme               *runtime.Scheme
	installGroveCRDs     bool
	installSchedulerCRDs bool

	// Controllers
	controllers    map[ControllerType]bool
	allControllers bool
	webhooks       map[WebhookType]bool

	// RBAC
	installRBAC    bool
	customRBAC     []rbacv1.PolicyRule
	serviceAccount string
	webhookOptions envtest.WebhookInstallOptions
	// Namespaces
	namespaces map[string]*corev1.Namespace

	// Pre-created objects
	objects []client.Object
}

// NewEnvBuilder creates a new environment builder with sensible defaults
func NewEnvBuilder(t *testing.T) *EnvBuilder {
	return &EnvBuilder{
		t:              t,
		crds:           []*apiextensionsv1.CustomResourceDefinition{},
		controllers:    make(map[ControllerType]bool),
		webhooks:       make(map[WebhookType]bool),
		namespaces:     make(map[string]*corev1.Namespace),
		webhookOptions: envtest.WebhookInstallOptions{},
		objects:        []client.Object{},
		scheme:         groveclient.Scheme, // Use Grove's production scheme
		// Enable common requirements by default
		installGroveCRDs:     true,
		installSchedulerCRDs: true,
		installRBAC:          true,
		serviceAccount:       "grove-operator-test",
	}
}

func (b *EnvBuilder) WithCRDs(crds ...*apiextensionsv1.CustomResourceDefinition) *EnvBuilder {
	for _, crd := range crds {
		b.crds = append(b.crds, crd)
	}
	return b
}

// WithController enables a specific controllers
func (b *EnvBuilder) WithController(controllerType ControllerType) *EnvBuilder {
	b.controllers[controllerType] = true
	return b
}

// WithValidationWebhook enables validation webhooks
func (b *EnvBuilder) WithValidationWebhook() *EnvBuilder {
	b.webhookOptions.ValidatingWebhooks = append(b.webhookOptions.ValidatingWebhooks, b.getPGSWebhookValidationConfig())
	b.objects = append(b.objects, b.getPGSWebhookValidationConfig())
	b.webhooks[WebhookValidation] = true
	return b
}

// WithMutationWebhook enables mutation webhooks
func (b *EnvBuilder) WithMutationWebhook() *EnvBuilder {
	b.webhookOptions.MutatingWebhooks = append(b.webhookOptions.MutatingWebhooks, b.getPGSWebhookMutationConfig())
	b.webhooks[WebhookMutation] = true
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
func (b *EnvBuilder) Build() (*TestEnv, error) {
	// Create context
	b.ctx, b.cancel = context.WithCancel(context.Background())
	operatorCrds, err := getOperatorCRDs()
	if err != nil {
		return nil, err
	}
	b.crds = append(b.crds, operatorCrds...)
	schedulerCrds, err := getSchedulerCRDs()
	if err != nil {
		return nil, err
	}
	b.crds = append(b.crds, schedulerCrds...)
	// Setup envtest environment
	b.env = &envtest.Environment{
		CRDs:                  b.crds,
		Scheme:                b.scheme,
		WebhookInstallOptions: b.webhookOptions,
	}

	// Register cleanup
	b.t.Cleanup(func() {
		b.cleanup()
	})

	return &TestEnv{
		T:       b.t,
		Client:  nil, // Will be created in Start()
		Manager: nil, // Will be created in Start()
		Ctx:     b.ctx,
		Objects: b.objects,

		// Internal fields for lifecycle management
		env:    b.env,
		cancel: b.cancel,

		namespaceConfigs: b.namespaces,
		controllers:      b.controllers,
		webhooks:         b.webhooks,

		// RBAC configuration
		installRBAC:    b.installRBAC,
		customRBAC:     b.customRBAC,
		serviceAccount: b.serviceAccount,
	}, nil
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
			Rules:                   defaultWebhookRules(),
			FailurePolicy:           ptr.To(admissionregistrationv1.Fail),
			MatchPolicy:             ptr.To(admissionregistrationv1.Exact),
			SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
			TimeoutSeconds:          ptr.To[int32](10),
			AdmissionReviewVersions: []string{"v1"},
		}},
	}
	return webhookConfig
}

func getSerivceForWebhook(serviceName string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: "grove-operator-test",
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": "grove-operator-test"},
			Ports: []corev1.ServicePort{{
				Name:       "webhook",
				Port:       443,
				TargetPort: intstr.FromInt(9443),
			}},
		},
	}
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
			Rules:                   defaultWebhookRules(),
			FailurePolicy:           ptr.To(admissionregistrationv1.Fail),
			SideEffects:             ptr.To(admissionregistrationv1.SideEffectClassNone),
			TimeoutSeconds:          ptr.To[int32](10),
			AdmissionReviewVersions: []string{"v1"},
		}},
	}
	return &webhookConfig
}
