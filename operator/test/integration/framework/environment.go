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
	"errors"
	"fmt"
	"testing"
	"time"

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
	t      *testing.T
}

// NewEnvironmentSetup creates a new environment setup helper
func NewEnvironmentSetup(ctx context.Context, env *envtest.Environment, t *testing.T) *EnvironmentSetup {
	return &EnvironmentSetup{
		env: env,
		ctx: ctx,
		t:   t,
	}
}

// InitializeLogger sets up the controllers-runtime logger
func (es *EnvironmentSetup) InitializeLogger() {
	ctrl.SetLogger(grovelogger.MustNewLogger(true, configv1alpha1.DebugLevel, configv1alpha1.LogFormatJSON))
}

// StartControlPlane starts the envtest control plane
func (es *EnvironmentSetup) StartControlPlane() error {
	es.t.Logf("Starting envtest control plane with %d CRDs", len(es.env.CRDs))
	start := time.Now()

	cfg, err := es.env.Start()
	if err != nil {
		return fmt.Errorf("failed to start envtest: %w", err)
	}
	es.env.Config = cfg

	duration := time.Since(start)
	es.t.Logf("Envtest control plane started successfully (took %v)", duration)
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
	if len(namespaces) == 0 {
		return nil
	}
	es.t.Logf("Creating %d required namespaces", len(namespaces))
	nsManager := NewNamespaceManager(es.ctx, es.client, namespaces, es.t)
	if err := nsManager.CreateAll(); err != nil {
		return err
	}
	es.t.Logf("All required namespaces created successfully")
	return nil
}

// CreateManager creates the controllers manager with webhook support if needed
func (es *EnvironmentSetup) CreateManager(webhooks map[WebhookType]bool, webhookOptions envtest.WebhookInstallOptions) (manager.Manager, error) {
	es.t.Logf("Creating controller manager with %d webhooks", len(webhooks))

	// Add webhook server configuration if webhooks are enabled
	managerOpts := ctrl.Options{
		Scheme:                 es.env.Scheme,
		LeaderElection:         false, // Disable leader election in tests
		HealthProbeBindAddress: "0",
	}
	if len(webhooks) > 0 {
		es.t.Logf("Configuring webhook server: port=%d, host=%s",
			webhookOptions.LocalServingPort, webhookOptions.LocalServingHost)
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
	es.t.Logf("Controller manager created successfully")
	return mgr, nil
}

// RegisterComponents registers controllers and webhooks with the manager
func (es *EnvironmentSetup) RegisterComponents(mgr manager.Manager, controllers map[ControllerType]bool, webhooks map[WebhookType]bool) error {
	es.t.Logf("Registering components: %d controllers, %d webhooks", len(controllers), len(webhooks))
	operatorCfg := defaultTestOperatorConfig()

	if len(controllers) > 0 {
		es.t.Logf("Registering controllers: %v", es.getControllerNames(controllers))
		controllerManager := NewControllerManager(mgr, controllers, operatorCfg, es.t)
		if err := controllerManager.RegisterAll(); err != nil {
			return err
		}
		es.t.Logf("All controllers registered successfully")
	}

	if len(webhooks) > 0 {
		es.t.Logf("Registering webhooks: %v", es.getWebhookNames(webhooks))
		webhookManager := NewWebhookManager(mgr, webhooks, es.t)
		if err := webhookManager.RegisterAll(); err != nil {
			return err
		}
		es.t.Logf("All webhooks registered successfully")
	}

	es.t.Logf("Component registration completed")
	return nil
}

// StartManager starts the manager and waits for cache sync
func (es *EnvironmentSetup) StartManager(mgr manager.Manager) error {
	es.t.Logf("Starting controller manager in background")
	go func() {
		_ = mgr.Start(es.ctx)
	}()

	es.t.Logf("Waiting for controller cache to sync")
	if !mgr.GetCache().WaitForCacheSync(es.ctx) {
		return errors.New("timed out waiting for cache to sync")
	}

	es.t.Logf("Controller cache synced successfully")
	return nil
}

// getControllerNames returns a slice of controller names for logging
func (es *EnvironmentSetup) getControllerNames(controllers map[ControllerType]bool) []string {
	var names []string
	for controllerType := range controllers {
		names = append(names, string(controllerType))
	}
	return names
}

// getWebhookNames returns a slice of webhook names for logging
func (es *EnvironmentSetup) getWebhookNames(webhooks map[WebhookType]bool) []string {
	var names []string
	for webhookType := range webhooks {
		names = append(names, string(webhookType))
	}
	return names
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
