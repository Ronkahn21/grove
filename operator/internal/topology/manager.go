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

package topology

import (
	"context"
	"fmt"
	"sort"

	configv1alpha1 "github.com/ai-dynamo/grove/operator/api/config/v1alpha1"
	corev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"

	kaitopologyv1alpha1 "github.com/NVIDIA/KAI-scheduler/pkg/apis/kai/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var logger = ctrl.Log.WithName("topology-manager")

// EnsureTopology creates or updates the ClusterTopology CR from configuration
func EnsureTopology(ctx context.Context, client client.Client, name string, levels []configv1alpha1.TopologyLevel) error {
	topology := &corev1alpha1.ClusterTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1alpha1.ClusterTopologySpec{
			Levels: convertTopologyLevels(levels),
		},
	}

	err := upsertClusterTopology(ctx, client, topology, name)
	if err != nil {
		return err
	}
	return ensureKAITopology(ctx, client, topology)
}

func upsertClusterTopology(ctx context.Context, client client.Client, topology *corev1alpha1.ClusterTopology, name string) error {
	err := client.Create(ctx, topology)
	if err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create ClusterTopology: %w", err)
		}
		existing := &corev1alpha1.ClusterTopology{}
		if err := client.Get(ctx, types.NamespacedName{Name: name}, existing); err != nil {
			return fmt.Errorf("failed to get existing ClusterTopology: %w", err)
		}
		existing.Spec = topology.Spec
		if err := client.Update(ctx, existing); err != nil {
			return fmt.Errorf("failed to update ClusterTopology: %w", err)
		}
		logger.Info("cluster topology updated successfully", "name", name)
	} else {
		logger.Info("cluster topology created successfully", "name", name)
	}
	return nil
}

// convertTopologyLevels converts configuration topology levels to core API topology levels
func convertTopologyLevels(levels []configv1alpha1.TopologyLevel) []corev1alpha1.TopologyLevel {
	result := make([]corev1alpha1.TopologyLevel, len(levels))
	for i, level := range levels {
		result[i] = corev1alpha1.TopologyLevel{
			Domain: corev1alpha1.TopologyDomain(level.Domain),
			Key:    level.Key,
		}
	}
	return result
}

// convertClusterTopologyToKai converts ClusterTopology to KAI Topology with sorted levels
func convertClusterTopologyToKai(clusterTopology *corev1alpha1.ClusterTopology) *kaitopologyv1alpha1.Topology {
	sortedLevels := make([]corev1alpha1.TopologyLevel, len(clusterTopology.Spec.Levels))
	copy(sortedLevels, clusterTopology.Spec.Levels)

	sort.Slice(sortedLevels, func(i, j int) bool {
		return sortedLevels[i].Domain.Compare(sortedLevels[j].Domain) < 0
	})

	kaiLevels := make([]kaitopologyv1alpha1.TopologyLevel, len(sortedLevels))
	for i, level := range sortedLevels {
		kaiLevels[i] = kaitopologyv1alpha1.TopologyLevel{
			NodeLabel: level.Key,
		}
	}

	return &kaitopologyv1alpha1.Topology{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterTopology.Name,
		},
		Spec: kaitopologyv1alpha1.TopologySpec{
			Levels: kaiLevels,
		},
	}
}

// ensureKAITopology creates or updates the KAI Topology CR from ClusterTopology
func ensureKAITopology(ctx context.Context, client client.Client, clusterTopology *corev1alpha1.ClusterTopology) error {
	kaiTopology := convertClusterTopologyToKai(clusterTopology)

	if err := controllerutil.SetControllerReference(clusterTopology, kaiTopology, client.Scheme()); err != nil {
		return fmt.Errorf("failed to set owner reference for KAI Topology: %w", err)
	}

	err := client.Create(ctx, kaiTopology)
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			existing := &kaitopologyv1alpha1.Topology{}
			if err := client.Get(ctx, types.NamespacedName{Name: kaiTopology.Name}, existing); err != nil {
				return fmt.Errorf("failed to get existing KAI Topology: %w", err)
			}
			existing.Spec = kaiTopology.Spec
			if err := client.Update(ctx, existing); err != nil {
				return fmt.Errorf("failed to update KAI Topology: %w", err)
			}
			logger.Info("KAI topology updated successfully", "name", kaiTopology.Name)
			return nil
		}
		return fmt.Errorf("failed to create KAI Topology: %w", err)
	}

	logger.Info("KAI topology created successfully", "name", kaiTopology.Name)
	return nil
}
