//go:build e2e

package tests

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

import (
	"context"
	"testing"

	corev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestTopologyInfrastructure verifies that the operator creates ClusterTopology and KAI Topology CRs at startup.
// This test validates the basic topology infrastructure without deploying any workloads.
func TestTopologyInfrastructure(t *testing.T) {
	ctx := context.Background()

	clientset, _, dynamicClient, cleanup := prepareTestCluster(ctx, t, 0)
	defer cleanup()

	t.Log("Verifying ClusterTopology CR exists and has correct levels")

	expectedLevels := []corev1alpha1.TopologyLevel{
		{Domain: corev1alpha1.TopologyDomainZone, Key: "kubernetes.io/zone"},
		{Domain: corev1alpha1.TopologyDomainBlock, Key: "kubernetes.io/block"},
		{Domain: corev1alpha1.TopologyDomainRack, Key: "kubernetes.io/rack"},
		{Domain: corev1alpha1.TopologyDomainHost, Key: "kubernetes.io/hostname"},
	}

	if err := verifyClusterTopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedLevels); err != nil {
		t.Fatalf("Failed to verify ClusterTopology levels: %v", err)
	}

	t.Log("ClusterTopology verified successfully")

	t.Log("Verifying KAI Topology CR exists and has correct levels")

	expectedKeys := []string{
		"kubernetes.io/zone",
		"kubernetes.io/block",
		"kubernetes.io/rack",
		"kubernetes.io/hostname",
	}

	if err := verifyKAITopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedKeys); err != nil {
		t.Fatalf("Failed to verify KAI Topology levels: %v", err)
	}

	t.Log("KAI Topology verified successfully")

	t.Log("Verifying worker nodes have topology labels")

	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}

	workerCount := 0
	for _, node := range nodes.Items {
		if _, isControlPlane := node.Labels["node-role.kubernetes.io/control-plane"]; isControlPlane {
			continue
		}

		workerCount++

		// Verify zone label
		if zone, ok := node.Labels["kubernetes.io/zone"]; !ok || zone == "" {
			t.Errorf("Node %s missing kubernetes.io/zone label", node.Name)
		}

		// Verify block label
		if block, ok := node.Labels["kubernetes.io/block"]; !ok || block == "" {
			t.Errorf("Node %s missing kubernetes.io/block label", node.Name)
		}

		// Verify rack label
		if rack, ok := node.Labels["kubernetes.io/rack"]; !ok || rack == "" {
			t.Errorf("Node %s missing kubernetes.io/rack label", node.Name)
		}

		// hostname label should exist by default
		if hostname, ok := node.Labels["kubernetes.io/hostname"]; !ok || hostname == "" {
			t.Errorf("Node %s missing kubernetes.io/hostname label", node.Name)
		}
	}

	if workerCount == 0 {
		t.Fatal("No worker nodes found in cluster")
	}

	t.Logf("Successfully verified topology labels on %d worker nodes", workerCount)
	t.Log("Topology infrastructure test completed successfully")
}
