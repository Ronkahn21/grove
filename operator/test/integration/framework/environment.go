// /*
// Copyright 2025 The Grove Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package framework

import (
	"context"
	"fmt"

	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	grovelogger "github.com/NVIDIA/grove/operator/internal/logger"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// EnvironmentSetup handles low-level environment initialization
type EnvironmentSetup struct {
	env    *envtest.Environment
	client client.Client
	mgr    manager.Manager
	ctx    context.Context
}

// NewEnvironmentSetup creates a new environment setup helper
func NewEnvironmentSetup(ctx context.Context, env *envtest.Environment) *EnvironmentSetup {
	return &EnvironmentSetup{
		env: env,
		ctx: ctx,
	}
}

// InitializeLogger sets up the controllers-runtime logger
func (es *EnvironmentSetup) InitializeLogger() {
	ctrl.SetLogger(grovelogger.MustNewLogger(true, configv1alpha1.DebugLevel, configv1alpha1.LogFormatJSON))
}

// StartControlPlane starts the envtest control plane
func (es *EnvironmentSetup) StartControlPlane() error {
	cfg, err := es.env.Start()
	if err != nil {
		return fmt.Errorf("failed to start envtest: %w", err)
	}
	es.env.Config = cfg
	return nil
}

// SetupClient creates and configures the Kubernetes client
func (es *EnvironmentSetup) SetupClient() (client.Client, error) {
	kubeClient, err := client.New(es.env.Config, client.Options{Scheme: es.env.Scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}
	es.client = kubeClient
	return kubeClient, nil
}

// CreateRequiredNamespaces creates the configured namespaces
func (es *EnvironmentSetup) CreateRequiredNamespaces(namespaces map[string]*corev1.Namespace) error {
	nsManager := NewNamespaceManager(es.ctx, es.client, namespaces)
	if err := nsManager.CreateAll(); err != nil {
		return err
	}
	return nil
}

// CreateManager creates the controllers manager with webhook support if needed
func (es *EnvironmentSetup) CreateManager(webhooks map[WebhookType]bool, webhookOptions envtest.WebhookInstallOptions) (manager.Manager, error) {
	// Add webhook server configuration if webhooks are enabled
	managerOpts := ctrl.Options{
		Scheme:                 es.env.Scheme,
		LeaderElection:         false, // Disable leader election in tests
		HealthProbeBindAddress: "0",
	}
	if len(webhooks) > 0 {
		webhookServer := webhook.NewServer(webhook.Options{
			Port:    webhookOptions.LocalServingPort,
			Host:    webhookOptions.LocalServingHost,
			CertDir: webhookOptions.LocalServingCertDir,
		})
		managerOpts.WebhookServer = webhookServer
	}

	mgr, err := ctrl.NewManager(es.env.Config, managerOpts)
	if err != nil {
		return nil, err
	}
	es.mgr = mgr
	return mgr, nil
}

// RegisterComponents registers controllers and webhooks with the manager
func (es *EnvironmentSetup) RegisterComponents(mgr manager.Manager, controllers map[ControllerType]bool, webhooks map[WebhookType]bool) error {
	operatorCfg := defaultTestOperatorConfig()

	if len(controllers) > 0 {
		controllerManager := NewControllerManager(mgr, controllers, operatorCfg)
		if err := controllerManager.RegisterAll(); err != nil {
			return err
		}
	}

	if len(webhooks) > 0 {
		webhookManager := NewWebhookManager(mgr, webhooks)
		if err := webhookManager.RegisterAll(); err != nil {
			return err
		}
	}

	return nil
}

// StartManager starts the manager and waits for cache sync
func (es *EnvironmentSetup) StartManager(mgr manager.Manager) error {
	go func() {
		_ = mgr.Start(es.ctx) // Manager stopped - errors are logged by controller-runtime
	}()

	if !mgr.GetCache().WaitForCacheSync(es.ctx) {
		return fmt.Errorf("cache sync timeout")
	}
	return nil
}

// defaultTestOperatorConfig returns the default operator configuration for tests
func defaultTestOperatorConfig() *configv1alpha1.OperatorConfiguration {
	return &configv1alpha1.OperatorConfiguration{
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
}
