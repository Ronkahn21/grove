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

// Test_TI1_TopologyInfrastructure verifies that the operator creates ClusterTopology and KAI Topology CRs at startup
// Scenario TI-1 (Topology Infrastructure Setup):
// 1. Verify ClusterTopology CR exists with correct 4-level hierarchy (zone, block, rack, host)
// 2. Verify KAI Topology CR exists with matching levels
// 3. Verify KAI Topology has owner reference to ClusterTopology
// 4. Verify worker nodes have topology labels
func Test_TOP_TI1_TopologyInfrastructure(t *testing.T) {
	ctx := context.Background()

	clientset, _, dynamicClient, cleanup := prepareTestCluster(ctx, t, 0)
	defer cleanup()

	logger.Info("1. Verify ClusterTopology CR exists with correct 4-level hierarchy")

	expectedLevels := []corev1alpha1.TopologyLevel{
		{Domain: corev1alpha1.TopologyDomainZone, Key: "kubernetes.io/zone"},
		{Domain: corev1alpha1.TopologyDomainBlock, Key: "kubernetes.io/block"},
		{Domain: corev1alpha1.TopologyDomainRack, Key: "kubernetes.io/rack"},
		{Domain: corev1alpha1.TopologyDomainHost, Key: "kubernetes.io/hostname"},
	}

	if err := verifyClusterTopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedLevels); err != nil {
		t.Fatalf("Failed to verify ClusterTopology levels: %v", err)
	}

	logger.Info("2. Verify KAI Topology CR exists with matching levels and owner reference")

	expectedKeys := []string{
		"kubernetes.io/zone",
		"kubernetes.io/block",
		"kubernetes.io/rack",
		"kubernetes.io/hostname",
	}

	if err := verifyKAITopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedKeys); err != nil {
		t.Fatalf("Failed to verify KAI Topology levels: %v", err)
	}

	logger.Info("3. Verify worker nodes have topology labels")

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

	logger.Infof("Successfully verified topology labels on %d worker nodes", workerCount)
	logger.Info("🎉 Topology Infrastructure test completed successfully!")
}

// Test_TOP_BP1_MultipleCliquesWithDifferentConstraints tests PCS with multiple cliques having different topology constraints
// Scenario BP-1:
// 1. Deploy workload with PCS (no constraint) containing 2 cliques:
//   - worker-rack: packDomain=rack (3 pods)
//   - worker-block: packDomain=block (4 pods)
//
// 2. Verify all 7 pods are scheduled successfully
// 3. Verify worker-rack pods (3) are in the same rack
// 4. Verify worker-block pods (4) are in the same block
// 5. Verify different cliques can have independent topology constraints
func Test_TOP_BP1_MultipleCliquesWithDifferentConstraints(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 7-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 7)
	defer cleanup()

	expectedPods := 7 // worker-rack: 3 pods, worker-block: 4 pods
	tc := TestContext{
		T:             t,
		Ctx:           ctx,
		Clientset:     clientset,
		RestConfig:    restConfig,
		DynamicClient: dynamicClient,
		Namespace:     "default",
		Timeout:       defaultPollTimeout,
		Interval:      defaultPollInterval,
		Workload: &WorkloadConfig{
			Name:         "workload7",
			YAMLPath:     "../yaml/workload7.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload7 (BP-1: multiple cliques with different constraints)")
	if _, err := deployAndVerifyWorkload(tc); err != nil {
		t.Fatalf("Failed to deploy workload: %v", err)
	}

	logger.Info("3. Wait for all pods to be scheduled and running")
	if err := waitForPodsReady(tc, expectedPods); err != nil {
		t.Fatalf("Failed to wait for pods ready: %v", err)
	}

	logger.Info("4. Verify worker-rack pods (3) are in the same rack")
	rackPods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload7-0-worker-rack")
	if err != nil {
		t.Fatalf("Failed to get worker-rack pods: %v", err)
	}
	if len(rackPods) != 3 {
		t.Fatalf("Expected 3 worker-rack pods, got %d", len(rackPods))
	}

	if err := verifyPodsInSameTopologyDomain(tc, rackPods, "kubernetes.io/rack"); err != nil {
		t.Fatalf("Failed to verify worker-rack pods in same rack: %v", err)
	}

	logger.Info("5. Verify worker-block pods (4) are in the same block")
	blockPods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload7-0-worker-block")
	if err != nil {
		t.Fatalf("Failed to get worker-block pods: %v", err)
	}
	if len(blockPods) != 4 {
		t.Fatalf("Expected 4 worker-block pods, got %d", len(blockPods))
	}

	if err := verifyPodsInSameTopologyDomain(tc, blockPods, "kubernetes.io/block"); err != nil {
		t.Fatalf("Failed to verify worker-block pods in same block: %v", err)
	}

	logger.Info("🎉 BP-1: Multiple Cliques with Different Constraints test completed successfully!")
}

// Test_TOP_SP1_FullHierarchyWithCascadingConstraints tests complete PCS → PCSG → PCLQ hierarchy
// Scenario SP-1:
// 1. Deploy workload with full 3-level hierarchy:
//   - PCS: packDomain=block
//   - PCSG: packDomain=rack (stricter than block)
//   - PodCliques (prefill, decode): packDomain=host (strictest)
//
// 2. Verify all 4 pods are scheduled successfully
// 3. Verify all pods are on the same host (strictest constraint wins)
// 4. Verify constraint inheritance and override behavior
func Test_TOP_SP1_FullHierarchyWithCascadingConstraints(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize an 8-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 8)
	defer cleanup()

	expectedPods := 8 // 2 PCSG replicas × (prefill: 2 pods + decode: 2 pods)
	tc := TestContext{
		T:             t,
		Ctx:           ctx,
		Clientset:     clientset,
		RestConfig:    restConfig,
		DynamicClient: dynamicClient,
		Namespace:     "default",
		Timeout:       defaultPollTimeout,
		Interval:      defaultPollInterval,
		Workload: &WorkloadConfig{
			Name:         "workload8",
			YAMLPath:     "../yaml/workload8.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload8 (SP-1: full 3-level hierarchy with cascading constraints)")
	if _, err := deployAndVerifyWorkload(tc); err != nil {
		t.Fatalf("Failed to deploy workload: %v", err)
	}

	logger.Info("3. Wait for all pods to be scheduled and running")
	if err := waitForPodsReady(tc, expectedPods); err != nil {
		t.Fatalf("Failed to wait for pods ready: %v", err)
	}

	logger.Info("4. Verify PCSG replica 0 prefill pods (2) are on same host (PCLQ constraint)")
	prefill0Pods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload8-0-inference-group-0-prefill")
	if err != nil {
		t.Fatalf("Failed to get prefill-0 pods: %v", err)
	}
	if len(prefill0Pods) != 2 {
		t.Fatalf("Expected 2 prefill-0 pods, got %d", len(prefill0Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, prefill0Pods, "kubernetes.io/hostname"); err != nil {
		t.Fatalf("Failed to verify prefill-0 pods on same host: %v", err)
	}

	logger.Info("5. Verify PCSG replica 0 decode pods (2) are on same host (PCLQ constraint)")
	decode0Pods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload8-0-inference-group-0-decode")
	if err != nil {
		t.Fatalf("Failed to get decode-0 pods: %v", err)
	}
	if len(decode0Pods) != 2 {
		t.Fatalf("Expected 2 decode-0 pods, got %d", len(decode0Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, decode0Pods, "kubernetes.io/hostname"); err != nil {
		t.Fatalf("Failed to verify decode-0 pods on same host: %v", err)
	}

	logger.Info("6. Verify PCSG replica 1 prefill pods (2) are on same host (PCLQ constraint)")
	prefill1Pods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload8-0-inference-group-1-prefill")
	if err != nil {
		t.Fatalf("Failed to get prefill-1 pods: %v", err)
	}
	if len(prefill1Pods) != 2 {
		t.Fatalf("Expected 2 prefill-1 pods, got %d", len(prefill1Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, prefill1Pods, "kubernetes.io/hostname"); err != nil {
		t.Fatalf("Failed to verify prefill-1 pods on same host: %v", err)
	}

	logger.Info("7. Verify PCSG replica 1 decode pods (2) are on same host (PCLQ constraint)")
	decode1Pods, err := getPodsWithLabel(tc, "grove.io/podclique", "workload8-0-inference-group-1-decode")
	if err != nil {
		t.Fatalf("Failed to get decode-1 pods: %v", err)
	}
	if len(decode1Pods) != 2 {
		t.Fatalf("Expected 2 decode-1 pods, got %d", len(decode1Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, decode1Pods, "kubernetes.io/hostname"); err != nil {
		t.Fatalf("Failed to verify decode-1 pods on same host: %v", err)
	}

	logger.Info("8. Verify all PCSG replica 0 pods are in same rack (PCSG constraint)")
	pcsg0Pods, err := getPodsWithLabel(tc, "grove.io/podcliquescalinggroup-replica-index", "0")
	if err != nil {
		t.Fatalf("Failed to get PCSG replica 0 pods: %v", err)
	}
	if len(pcsg0Pods) != 4 {
		t.Fatalf("Expected 4 PCSG replica 0 pods, got %d", len(pcsg0Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, pcsg0Pods, "kubernetes.io/rack"); err != nil {
		t.Fatalf("Failed to verify PCSG replica 0 pods in same rack: %v", err)
	}

	logger.Info("9. Verify all PCSG replica 1 pods are in same rack (PCSG constraint)")
	pcsg1Pods, err := getPodsWithLabel(tc, "grove.io/podcliquescalinggroup-replica-index", "1")
	if err != nil {
		t.Fatalf("Failed to get PCSG replica 1 pods: %v", err)
	}
	if len(pcsg1Pods) != 4 {
		t.Fatalf("Expected 4 PCSG replica 1 pods, got %d", len(pcsg1Pods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, pcsg1Pods, "kubernetes.io/rack"); err != nil {
		t.Fatalf("Failed to verify PCSG replica 1 pods in same rack: %v", err)
	}

	logger.Info("10. Verify all pods are in same block (PCS constraint)")
	allPods, err := getPodsWithLabel(tc, "app.kubernetes.io/part-of", "workload8")
	if err != nil {
		t.Fatalf("Failed to get workload pods: %v", err)
	}
	if len(allPods) != expectedPods {
		t.Fatalf("Expected %d pods, got %d", expectedPods, len(allPods))
	}
	if err := verifyPodsInSameTopologyDomain(tc, allPods, "kubernetes.io/block"); err != nil {
		t.Fatalf("Failed to verify all pods in same block: %v", err)
	}

	logger.Info("🎉 SP-1: Full Hierarchy with Cascading Constraints test completed successfully!")
}
