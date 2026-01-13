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
	"fmt"
	"testing"

	corev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	"github.com/ai-dynamo/grove/operator/e2e/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test_TI1_TopologyInfrastructure verifies that the operator creates ClusterTopology and KAI Topology CRs at startup
// Scenario TI-1 (Topology Infrastructure Setup):
// 1. Verify ClusterTopology CR exists with the correct 4-level hierarchy (zone, block, rack, host)
// 2. Verify KAI Topology CR exists with matching levels
// 3. Verify KAI Topology has owner reference to ClusterTopology
// 4. Verify worker nodes have topology labels
func Test_TAS_TI1_TopologyInfrastructure(t *testing.T) {
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

	if err := utils.VerifyClusterTopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedLevels, logger); err != nil {
		t.Fatalf("Failed to verify ClusterTopology levels: %v", err)
	}

	logger.Info("2. Verify KAI Topology CR exists with matching levels and owner reference")

	expectedKeys := []string{
		"kubernetes.io/zone",
		"kubernetes.io/block",
		"kubernetes.io/rack",
		"kubernetes.io/hostname",
	}

	if err := utils.VerifyKAITopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedKeys, logger); err != nil {
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

// Test_TAS_BP1_MultipleCliquesWithDifferentConstraints tests PCS with multiple cliques having different topology constraints
// Scenario BP-1:
// 1. Deploy workload with PCS (no constraint) containing 2 cliques:
//   - worker-rack: packDomain=rack (3 pods)
//   - worker-block: packDomain=block (4 pods)
//
// 2. Verify all 7 pods are scheduled successfully
// 3. Verify worker-rack pods (3) are in the same rack
// 4. Verify worker-block pods (4) are in the same block
// 5. Verify different cliques can have independent topology constraints
func Test_TAS_BP1_MultipleCliquesWithDifferentConstraints(t *testing.T) {
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
			Name:         "top-indep-clq",
			YAMLPath:     "../yaml/top-indep-clq.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload7 (BP-1: multiple cliques with different constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify worker-rack pods (3) are in the same rack")
	rackPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-indep-clq-0-worker-rack")
	if len(rackPods) != 3 {
		t.Fatalf("Expected 3 worker-rack pods, got %d", len(rackPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, rackPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify worker-rack pods in same rack: %v", err)
	}

	logger.Info("4. Verify worker-block pods (4) are in the same block")
	blockPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-indep-clq-0-worker-block")
	if len(blockPods) != 4 {
		t.Fatalf("Expected 4 worker-block pods, got %d", len(blockPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, blockPods, "kubernetes.io/block", logger); err != nil {
		t.Fatalf("Failed to verify worker-block pods in same block: %v", err)
	}

	logger.Info("🎉 BP-1: Multiple Cliques with Different Constraints test completed successfully!")
}

// Test_TAS_SP1_FullHierarchyWithCascadingConstraints tests complete PCS → PCSG → PCLQ hierarchy
// Scenario SP-1:
// 1. Deploy workload with full 3-level hierarchy:
//   - PCS: packDomain=block
//   - PCSG: packDomain=rack (stricter than block)
//   - PodCliques (prefill, decode): packDomain=host (strictest)
//
// 2. Verify all 8 pods are scheduled successfully
// 3. Verify all pods are on the same host (strictest constraint wins)
// 4. Verify constraint inheritance and override behavior
func Test_TAS_SP1_FullHierarchyWithCascadingConstraints(t *testing.T) {
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
			Name:         "top-hierarchy",
			YAMLPath:     "../yaml/top-hierarchy.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload8 (SP-1: full 3-level hierarchy with cascading constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify PCSG replica 0 prefill pods (2) are on same host (PCLQ constraint)")
	prefill0Pods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-hierarchy-0-inference-group-0-prefill")
	if len(prefill0Pods) != 2 {
		t.Fatalf("Expected 2 prefill-0 pods, got %d", len(prefill0Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, prefill0Pods, "kubernetes.io/hostname", logger); err != nil {
		t.Fatalf("Failed to verify prefill-0 pods on same host: %v", err)
	}

	logger.Info("4. Verify PCSG replica 0 decode pods (2) are on same host (PCLQ constraint)")
	decode0Pods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-hierarchy-0-inference-group-0-decode")
	if len(decode0Pods) != 2 {
		t.Fatalf("Expected 2 decode-0 pods, got %d", len(decode0Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, decode0Pods, "kubernetes.io/hostname", logger); err != nil {
		t.Fatalf("Failed to verify decode-0 pods on same host: %v", err)
	}

	logger.Info("5. Verify PCSG replica 1 prefill pods (2) are on same host (PCLQ constraint)")
	prefill1Pods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-hierarchy-0-inference-group-1-prefill")
	if len(prefill1Pods) != 2 {
		t.Fatalf("Expected 2 prefill-1 pods, got %d", len(prefill1Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, prefill1Pods, "kubernetes.io/hostname", logger); err != nil {
		t.Fatalf("Failed to verify prefill-1 pods on same host: %v", err)
	}

	logger.Info("6. Verify PCSG replica 1 decode pods (2) are on same host (PCLQ constraint)")
	decode1Pods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-hierarchy-0-inference-group-1-decode")
	if len(decode1Pods) != 2 {
		t.Fatalf("Expected 2 decode-1 pods, got %d", len(decode1Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, decode1Pods, "kubernetes.io/hostname", logger); err != nil {
		t.Fatalf("Failed to verify decode-1 pods on same host: %v", err)
	}

	logger.Info("7. Verify all PCSG replica 0 pods are in same rack (PCSG constraint)")
	pcsg0Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup-replica-index", "0")
	if len(pcsg0Pods) != 4 {
		t.Fatalf("Expected 4 PCSG replica 0 pods, got %d", len(pcsg0Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, pcsg0Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify PCSG replica 0 pods in same rack: %v", err)
	}

	logger.Info("8. Verify all PCSG replica 1 pods are in same rack (PCSG constraint)")
	pcsg1Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup-replica-index", "1")
	if len(pcsg1Pods) != 4 {
		t.Fatalf("Expected 4 PCSG replica 1 pods, got %d", len(pcsg1Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, pcsg1Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify PCSG replica 1 pods in same rack: %v", err)
	}

	logger.Info("9. Verify all pods are in same block (PCS constraint)")
	if len(allPods) != expectedPods {
		t.Fatalf("Expected %d pods, got %d", expectedPods, len(allPods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/block", logger); err != nil {
		t.Fatalf("Failed to verify all pods in same block: %v", err)
	}

	logger.Info("🎉 SP-1: Full Hierarchy with Cascading Constraints test completed successfully!")
}

// deployWorkloadAndGetPods deploys workload, waits for pods, and returns the pod list
func deployWorkloadAndGetPods(tc TestContext, expectedPods int) ([]v1.Pod, error) {
	if _, err := deployAndVerifyWorkload(tc); err != nil {
		return nil, fmt.Errorf("failed to deploy workload: %w", err)
	}

	logger.Info("Wait for all pods to be scheduled and running")
	if err := utils.WaitForPodsReady(tc.Ctx, tc.Clientset, tc.Namespace, tc.getLabelSelector(), expectedPods, tc.Timeout, tc.Interval, logger); err != nil {
		return nil, fmt.Errorf("failed to wait for pods ready: %w", err)
	}

	logger.Info("Get all pods once for verification")
	podList, err := listPods(tc)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return podList.Items, nil
}

// Test_TAS_SP3_PCSGScalingWithTopologyConstraints tests PCSG scaling with topology constraints
// Scenario SP-3:
// 1. Deploy workload with PCSG scaling (3 replicas):
//   - PCS: packDomain=rack, minAvailable=1
//   - PCSG: replicas=3, packDomain=rack
//   - PodClique (worker): 2 pods per replica
//
// 2. Verify all 6 pods (3 PCSG replicas × 2 pods) are scheduled successfully
// 3. Verify each PCSG replica's pods are in the same rack
// 4. Verify PCSG scaling creates multiple TopologyConstraintGroups
// 5. Verify topology constraints work with PCSG-level scaling
func Test_TAS_SP3_PCSGScalingWithTopologyConstraints(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 6 // 3 PCSG replicas × 2 pods each
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
			Name:         "top-pcsg-scale",
			YAMLPath:     "../yaml/top-pcsg-scale.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload9 (SP-3: PCSG scaling with topology constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify PCSG replica 0 worker pods (2) are in same rack")
	pcsg0Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup-replica-index", "0")
	if len(pcsg0Pods) != 2 {
		t.Fatalf("Expected 2 PCSG replica 0 pods, got %d", len(pcsg0Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, pcsg0Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify PCSG replica 0 pods in same rack: %v", err)
	}

	logger.Info("4. Verify PCSG replica 1 worker pods (2) are in same rack")
	pcsg1Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup-replica-index", "1")
	if len(pcsg1Pods) != 2 {
		t.Fatalf("Expected 2 PCSG replica 1 pods, got %d", len(pcsg1Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, pcsg1Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify PCSG replica 1 pods in same rack: %v", err)
	}

	logger.Info("5. Verify PCSG replica 2 worker pods (2) are in same rack")
	pcsg2Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup-replica-index", "2")
	if len(pcsg2Pods) != 2 {
		t.Fatalf("Expected 2 PCSG replica 2 pods, got %d", len(pcsg2Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, pcsg2Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify PCSG replica 2 pods in same rack: %v", err)
	}

	logger.Info("6. Verify all pods respect PCS-level rack constraint")
	if len(allPods) != expectedPods {
		t.Fatalf("Expected %d pods, got %d", expectedPods, len(allPods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify all pods in same rack: %v", err)
	}

	logger.Info("🎉 SP-3: PCSG Scaling with Topology Constraints test completed successfully!")
}

// Test_TAS_EC1_InsufficientNodesForConstraint tests gang scheduling failure when topology constraint cannot be satisfied
// Scenario EC-1:
// 1. Deploy workload with rack constraint requesting 10 pods (exceeds rack capacity)
// 2. Verify all 10 pods remain in Pending state (no partial scheduling)
// 3. Verify NO pods are scheduled (all-or-nothing gang behavior)
// 4. Verify pod events show Unschedulable reason
func Test_TAS_EC1_InsufficientNodesForConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 10
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
			Name:         "top-insuffic",
			YAMLPath:     "../yaml/top-insuffic.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload10 (EC-1: insufficient nodes for rack constraint)")
	_, err := deployAndVerifyWorkload(tc)
	if err != nil {
		t.Fatalf("Failed to deploy workload: %v", err)
	}

	logger.Info("3. Verify all 10 pods remain in Pending state (no partial scheduling)")
	if err := verifyPodsArePendingWithUnschedulableEvents(tc, true, expectedPods); err != nil {
		t.Fatalf("Failed to verify pods are pending with unschedulable events: %v", err)
	}

	logger.Info("4. Verify NO pods are scheduled (all-or-nothing gang behavior)")
	pods, err := listPods(tc)
	if err != nil {
		t.Fatalf("Failed to list pods: %v", err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != v1.PodPending {
			t.Fatalf("Expected all pods to be Pending, but pod %s is in phase %s", pod.Name, pod.Status.Phase)
		}
		if pod.Spec.NodeName != "" {
			t.Fatalf("Expected pod %s to have no node assignment, but assigned to %s", pod.Name, pod.Spec.NodeName)
		}
	}

	logger.Info("🎉 EC-1: Insufficient Nodes for Constraint test completed successfully!")
}

// Test_TAS_MR1_MultiReplicaWithRackConstraint tests multi-replica PCS with per-replica topology packing
// Scenario MR-1:
// 1. Deploy workload with 2 PCS replicas, each with rack constraint (2 pods per replica)
// 2. Verify all 4 pods are scheduled successfully
// 3. Verify PCS replica 0 pods (2) are in same rack (per-replica packing)
// 4. Verify PCS replica 1 pods (2) are in same rack (per-replica packing)
// Note: We do NOT verify replicas are in different racks (spread constraints not supported)
func Test_TAS_MR1_MultiReplicaWithRackConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 4 // 2 PCS replicas × 2 pods each
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
			Name:         "top-multirep",
			YAMLPath:     "../yaml/top-multirep.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload11 (MR-1: multi-replica with rack constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify PCS replica 0 pods (2) are in same rack")
	replica0Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliqueset-replica-index", "0")
	if len(replica0Pods) != 2 {
		t.Fatalf("Expected 2 replica-0 pods, got %d", len(replica0Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, replica0Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify replica-0 pods in same rack: %v", err)
	}

	logger.Info("4. Verify PCS replica 1 pods (2) are in same rack")
	replica1Pods := utils.FilterPodsByLabel(allPods, "grove.io/podcliqueset-replica-index", "1")
	if len(replica1Pods) != 2 {
		t.Fatalf("Expected 2 replica-1 pods, got %d", len(replica1Pods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, replica1Pods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify replica-1 pods in same rack: %v", err)
	}

	logger.Info("🎉 MR-1: Multi-Replica with Rack Constraint test completed successfully!")
}

// Test_TAS_SP4_DisaggregatedInferenceMultiplePCSGs tests disaggregated inference with multiple PCSGs
// Scenario SP-4:
// 1. Deploy workload with 2 PCSGs (decoder, prefill) + standalone router:
//   - PCS: packDomain=block (all 10 pods in same block)
//   - Decoder PCSG: replicas=2, minAvaileale=1 (each replica's 2 pods in same rack)
//   - Prefill PCSG: replicas=2, minAvailable=1, (each replica's 2 pods in same rack)
//   - Router: standalone, 2 pods (no PCSG, no topology constraint)
//
// 2. Verify all 10 pods are scheduled successfully
// 3. Verify block-level constraint covers all pods
// 4. Verify each PCSG replica respects rack-level constraint independently
// 5. Verify router pods have no PCSG replica index label
func Test_TAS_SP4_DisaggregatedInferenceMultiplePCSGs(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 10 // decoder (2×2) + prefill (2×2) + router (2)
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
			Name:         "top-disagg-inference",
			YAMLPath:     "../yaml/top-disagg-inference.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (SP-4: disaggregated inference with multiple PCSGs with minAvailable 1)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify block-level constraint (all 10 pods in same block)")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/block", logger); err != nil {
		t.Fatalf("Failed to verify all pods in same block: %v", err)
	}

	logger.Info("4. Verify decoder PCSG replica-0 (2 pods in same rack)")
	decoderReplica0 := utils.FilterPodsByLabel(
		utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-disagg-inference-0-decoder"),
		"grove.io/podcliquescalinggroup-replica-index", "0")
	if len(decoderReplica0) != 2 {
		t.Fatalf("Expected 2 decoder replica-0 pods, got %d", len(decoderReplica0))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, decoderReplica0, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify decoder replica-0 pods in same rack: %v", err)
	}

	logger.Info("5. Verify decoder PCSG replica-1 (2 pods in same rack)")
	decoderReplica1 := utils.FilterPodsByLabel(
		utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-disagg-inference-0-decoder"),
		"grove.io/podcliquescalinggroup-replica-index", "1")
	if len(decoderReplica1) != 2 {
		t.Fatalf("Expected 2 decoder replica-1 pods, got %d", len(decoderReplica1))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, decoderReplica1, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify decoder replica-1 pods in same rack: %v", err)
	}

	logger.Info("6. Verify prefill PCSG replica-0 (2 pods in same rack)")
	prefillReplica0 := utils.FilterPodsByLabel(
		utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-disagg-inference-0-prefill"),
		"grove.io/podcliquescalinggroup-replica-index", "0")
	if len(prefillReplica0) != 2 {
		t.Fatalf("Expected 2 prefill replica-0 pods, got %d", len(prefillReplica0))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, prefillReplica0, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify prefill replica-0 pods in same rack: %v", err)
	}

	logger.Info("7. Verify prefill PCSG replica-1 (2 pods in same rack)")
	prefillReplica1 := utils.FilterPodsByLabel(
		utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-disagg-inference-0-prefill"),
		"grove.io/podcliquescalinggroup-replica-index", "1")
	if len(prefillReplica1) != 2 {
		t.Fatalf("Expected 2 prefill replica-1 pods, got %d", len(prefillReplica1))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, prefillReplica1, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify prefill replica-1 pods in same rack: %v", err)
	}

	logger.Info("8. Verify router pods (2 standalone, no PCSG label)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-disagg-inference-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("🎉 SP-4: Disaggregated Inference with Multiple PCSGs test completed successfully!")
}

// Test_TAS_SL1_PCSOnlyConstraint tests constraint only at PCS level with no PCSG/PCLQ constraints
// Scenario SL-1:
// 1. Deploy workload with constraint only at PCS level (packDomain: rack)
// 2. PCSG and PCLQs have NO explicit constraints
// 3. Verify all 4 pods (2 PCSG workers + 2 router) in same rack via inheritance
func Test_TAS_SL1_PCSOnlyConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 4 // 2 PCSG workers + 2 router standalone
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
			Name:         "top-sl-pcs-only",
			YAMLPath:     "../yaml/top-sl-pcs-only.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (SL-1: PCS-only constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all 4 pods in same rack (inherited from PCS)")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify all pods in same rack: %v", err)
	}

	logger.Info("4. Verify PCSG worker pods (2 total, 1 per replica)")
	workerPods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-sl-pcs-only-0-workers")
	if len(workerPods) != 2 {
		t.Fatalf("Expected 2 worker pods, got %d", len(workerPods))
	}

	logger.Info("5. Verify router pods (2 standalone)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-sl-pcs-only-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("🎉 SL-1: PCS-Only Constraint test completed successfully!")
}

// Test_TAS_SL2_PCSGOnlyConstraint tests constraint only at PCSG level with no PCS/PCLQ constraints
// Scenario SL-2:
// 1. Deploy workload with constraint only at PCSG level (packDomain: rack)
// 2. PCS and PCLQs have NO explicit constraints
// 3. Verify PCSG worker pods (2 total) respect rack constraint
// 4. Router pods (2 standalone) are unconstrained
func Test_TAS_SL2_PCSGOnlyConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 4 // 2 PCSG workers + 2 router standalone
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
			Name:         "top-sl-pcsg-only",
			YAMLPath:     "../yaml/top-sl-pcsg-only.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (SL-2: PCSG-only constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify PCSG worker pods (2 total, 1 per replica) in same rack")
	workerPods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "top-sl-pcsg-only-0-workers")
	if len(workerPods) != 2 {
		t.Fatalf("Expected 2 worker pod, got %d", len(workerPods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, workerPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify worker pods in same rack: %v", err)
	}

	logger.Info("4. Verify router pods (2 standalone, unconstrained)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "top-sl-pcsg-only-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("🎉 SL-2: PCSG-Only Constraint test completed successfully!")
}

// Test_TAS_PC1_HostLevelConstraint tests PCLQ-only constraint with host-level packing
// Scenario PC-1:
// 1. Deploy workload with constraint only at PCLQ level (packDomain: host)
// 2. PCS has NO explicit constraint
// 3. Verify all 2 pods on same host (strictest constraint)
func Test_TAS_PC1_HostLevelConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 2
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
			Name:         "top-host-level",
			YAMLPath:     "../yaml/top-host-level.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (PC-1: PCLQ-only host constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all pods on same host")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/hostname", logger); err != nil {
		t.Fatalf("Failed to verify pods on same host: %v", err)
	}

	// Additional check: verify both pods have same node name
	if len(allPods) != 2 {
		t.Fatalf("Expected 2 pods, got %d", len(allPods))
	}
	if allPods[0].Spec.NodeName != allPods[1].Spec.NodeName {
		t.Fatalf("Pods not on same node: %s vs %s", allPods[0].Spec.NodeName, allPods[1].Spec.NodeName)
	}

	logger.Info("🎉 PC-1: Host-Level Constraint test completed successfully!")
}
