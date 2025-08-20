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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

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

	envSetup := NewEnvironmentSetup(te.Ctx, te.env)

	envSetup.InitializeLogger()
	te.T.Logf("Initialized controllers-runtime logger with debug level")

	if err := envSetup.StartControlPlane(); err != nil {
		return err
	}

	kubeClient, err := envSetup.SetupClient()
	if err != nil {
		return err
	}
	te.Client = kubeClient

	if err := envSetup.CreateRequiredNamespaces(te.namespaceConfigs); err != nil {
		return err
	}

	if err := te.setupAndStartManager(envSetup); err != nil {
		return err
	}

	te.started = true
	return nil
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

// setupAndStartManager creates, configures, and starts the manager if needed
func (te *TestEnv) setupAndStartManager(envSetup *EnvironmentSetup) error {
	te.T.Logf("Starting manager")
	mgr, err := envSetup.CreateManager(te.webhooks, te.env.WebhookInstallOptions)
	if err != nil {
		return err
	}
	te.T.Logf("Manager created")
	te.Manager = mgr

	te.T.Logf("registering components")
	if err = envSetup.RegisterComponents(te.Manager, te.controllers, te.webhooks); err != nil {
		return err
	}
	te.T.Logf("Components registered successfully")
	te.T.Logf("Starting manager")
	if err := envSetup.StartManager(te.Manager); err != nil {
		return err
	}
	te.T.Logf("Manager started successfully")
	// Switch to manager's client after startup
	te.Client = te.Manager.GetClient()
	return nil
}
