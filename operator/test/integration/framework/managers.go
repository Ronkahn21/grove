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
	"time"

	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/controller/podclique"
	"github.com/NVIDIA/grove/operator/internal/controller/podcliquescalinggroup"
	"github.com/NVIDIA/grove/operator/internal/controller/podgangset"
	"github.com/NVIDIA/grove/operator/internal/webhook/admission/pgs/defaulting"
	"github.com/NVIDIA/grove/operator/internal/webhook/admission/pgs/validation"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// ControllerManager handles controller registration and lifecycle
type ControllerManager struct {
	mgr         manager.Manager
	controllers map[ControllerType]bool
	config      *configv1alpha1.OperatorConfiguration
	t           *testing.T
}

// NewControllerManager creates a new controller manager
func NewControllerManager(mgr manager.Manager, controllers map[ControllerType]bool, config *configv1alpha1.OperatorConfiguration, t *testing.T) *ControllerManager {
	return &ControllerManager{
		mgr:         mgr,
		controllers: controllers,
		config:      config,
		t:           t,
	}
}

// RegisterAll registers all configured controllers with the manager
func (cm *ControllerManager) RegisterAll() error {
	cm.t.Logf("Registering %d controllers", len(cm.controllers))
	for controllerType := range cm.controllers {
		start := time.Now()
		if err := cm.registerController(controllerType); err != nil {
			cm.t.Logf("ERROR: Failed to register controller %s: %v", controllerType, err)
			return err
		}
		duration := time.Since(start)
		cm.t.Logf("Controller %s registered successfully (took %v)", controllerType, duration)
	}
	cm.t.Logf("All controllers registered successfully")
	return nil
}

func (cm *ControllerManager) registerController(controllerType ControllerType) error {
	switch controllerType {
	case ControllerPodGangSet:
		return cm.registerPodGangSetController()
	case ControllerPodClique:
		return cm.registerPodCliqueController()
	case ControllerScalingGroup:
		return cm.registerScalingGroupController()
	default:
		return fmt.Errorf("unknown controller type: %s", controllerType)
	}
}

func (cm *ControllerManager) registerPodGangSetController() error {
	cm.t.Logf("Creating PodGangSet reconciler with concurrency=%d", *cm.config.Controllers.PodGangSet.ConcurrentSyncs)
	reconciler := podgangset.NewReconciler(cm.mgr, cm.config.Controllers.PodGangSet)
	if err := reconciler.RegisterWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to register PodGangSet controller: %w", err)
	}
	return nil
}

func (cm *ControllerManager) registerPodCliqueController() error {
	cm.t.Logf("Creating PodClique reconciler with concurrency=%d", *cm.config.Controllers.PodClique.ConcurrentSyncs)
	reconciler := podclique.NewReconciler(cm.mgr, cm.config.Controllers.PodClique)
	if err := reconciler.RegisterWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to register PodClique controller: %w", err)
	}
	return nil
}

func (cm *ControllerManager) registerScalingGroupController() error {
	cm.t.Logf("Creating PodCliqueScalingGroup reconciler with concurrency=%d", *cm.config.Controllers.PodCliqueScalingGroup.ConcurrentSyncs)
	reconciler := podcliquescalinggroup.NewReconciler(cm.mgr, cm.config.Controllers.PodCliqueScalingGroup)
	if err := reconciler.RegisterWithManager(cm.mgr); err != nil {
		return fmt.Errorf("failed to register PodCliqueScalingGroup controller: %w", err)
	}
	return nil
}

// WebhookManager handles webhook registration and lifecycle
type WebhookManager struct {
	mgr      manager.Manager
	webhooks map[WebhookType]bool
	t        *testing.T
}

// NewWebhookManager creates a new webhook manager
func NewWebhookManager(mgr manager.Manager, webhooks map[WebhookType]bool, t *testing.T) *WebhookManager {
	return &WebhookManager{
		mgr:      mgr,
		webhooks: webhooks,
		t:        t,
	}
}

// RegisterAll registers all configured webhooks with the manager
func (wm *WebhookManager) RegisterAll() error {
	wm.t.Logf("Registering %d webhooks", len(wm.webhooks))
	for webhookType := range wm.webhooks {
		start := time.Now()
		if err := wm.registerWebhook(webhookType); err != nil {
			wm.t.Logf("ERROR: Failed to register webhook %s: %v", webhookType, err)
			return err
		}
		duration := time.Since(start)
		wm.t.Logf("Webhook %s registered successfully (took %v)", webhookType, duration)
	}
	wm.t.Logf("All webhooks registered successfully")
	return nil
}

func (wm *WebhookManager) registerWebhook(webhookType WebhookType) error {
	switch webhookType {
	case WebhookValidation:
		return wm.registerValidationWebhook()
	case WebhookMutation:
		return wm.registerMutationWebhook()
	default:
		return fmt.Errorf("unknown webhook type: %s", webhookType)
	}
}

func (wm *WebhookManager) registerValidationWebhook() error {
	wm.t.Logf("Creating validation webhook handler")
	validatingWebhook := validation.NewHandler(wm.mgr)
	if err := validatingWebhook.RegisterWithManager(wm.mgr); err != nil {
		return fmt.Errorf("failed to register validation webhook: %w", err)
	}
	return nil
}

func (wm *WebhookManager) registerMutationWebhook() error {
	wm.t.Logf("Creating mutation webhook handler")
	defaultingWebhook := defaulting.NewHandler(wm.mgr)
	if err := defaultingWebhook.RegisterWithManager(wm.mgr); err != nil {
		return fmt.Errorf("failed to register mutation webhook: %w", err)
	}
	return nil
}

// NamespaceManager handles namespace creation and cleanup
type NamespaceManager struct {
	client     client.Client
	ctx        context.Context
	namespaces map[string]*corev1.Namespace
	t          *testing.T
}

// NewNamespaceManager creates a new namespace manager
func NewNamespaceManager(ctx context.Context, client client.Client, namespaces map[string]*corev1.Namespace, t *testing.T) *NamespaceManager {
	return &NamespaceManager{
		client:     client,
		ctx:        ctx,
		namespaces: namespaces,
		t:          t,
	}
}

// CreateAll creates all configured namespaces
func (nm *NamespaceManager) CreateAll() error {
	for _, ns := range nm.namespaces {
		nm.t.Logf("Creating namespace: name=%s, labels=%v", ns.Name, ns.Labels)
		if err := nm.client.Create(nm.ctx, ns); err != nil {
			return fmt.Errorf("failed to create namespace %s: %w", ns.Name, err)
		}
		nm.t.Logf("Namespace %s created successfully", ns.Name)
	}
	return nil
}
