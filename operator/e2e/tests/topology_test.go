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
	"github.com/ai-dynamo/grove/operator/e2e/setup"
	"github.com/ai-dynamo/grove/operator/e2e/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// deployWorkloadAndGetPods deploys workload, waits for pods to be ready, and returns the pod list
func deployWorkloadAndGetPods(tc TestContext, expectedPods int) ([]v1.Pod, error) {
	if _, err := deployAndVerifyWorkload(tc); err != nil {
		return nil, fmt.Errorf("failed to deploy workload: %w", err)
	}

	logger.Info("Wait for all pods to be scheduled and running")
	if err := utils.WaitForPods(tc.Ctx, tc.RestConfig, []string{tc.Namespace}, tc.getLabelSelector(), expectedPods, tc.Timeout, tc.Interval, logger); err != nil {
		return nil, fmt.Errorf("failed to wait for pods ready: %w", err)
	}

	logger.Info("Get all pods once for verification")
	podList, err := listPods(tc)
	if err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	return podList.Items, nil
}

// Test_TAS1_TopologyInfrastructure verifies that the operator creates ClusterTopology and KAI Topology CRs at startup
// 1. Verify ClusterTopology CR exists with the correct 4-level hierarchy (zone, block, rack, host)
// 2. Verify KAI Topology CR exists with matching levels
// 3. Verify KAI Topology has owner reference to ClusterTopology
// 4. Verify worker nodes have topology labels
func Test_TAS1_TopologyInfrastructure(t *testing.T) {
	ctx := context.Background()

	clientset, _, dynamicClient, cleanup := prepareTestCluster(ctx, t, 0)
	defer cleanup()

	logger.Info("1. Verify ClusterTopology CR exists with correct 4-level hierarchy")

	expectedLevels := []corev1alpha1.TopologyLevel{
		{Domain: corev1alpha1.TopologyDomainZone, Key: setup.TopologyLabelZone},
		{Domain: corev1alpha1.TopologyDomainBlock, Key: setup.TopologyLabelBlock},
		{Domain: corev1alpha1.TopologyDomainRack, Key: setup.TopologyLabelRack},
		{Domain: corev1alpha1.TopologyDomainHost, Key: setup.TopologyLabelHostname},
	}

	if err := utils.VerifyClusterTopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedLevels, logger); err != nil {
		t.Fatalf("Failed to verify ClusterTopology levels: %v", err)
	}

	logger.Info("2. Verify KAI Topology CR exists with matching levels and owner reference")

	expectedKeys := []string{
		setup.TopologyLabelZone,
		setup.TopologyLabelBlock,
		setup.TopologyLabelRack,
		setup.TopologyLabelHostname,
	}

	if err := utils.VerifyKAITopologyLevels(ctx, dynamicClient, corev1alpha1.DefaultClusterTopologyName, expectedKeys, logger); err != nil {
		t.Fatalf("Failed to verify KAI Topology levels: %v", err)
	}

	logger.Info("3. Verify worker nodes have topology labels")

	// Use label selector to get only worker nodes by role label
	workerLabelSelector := setup.GetWorkerNodeLabelSelector()
	nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{
		LabelSelector: workerLabelSelector,
	})
	if err != nil {
		t.Fatalf("Failed to list nodes: %v", err)
	}

	// Reuse expectedKeys from step 2 (same topology label keys)
	workerCount := len(nodes.Items)
	for _, node := range nodes.Items {
		for _, key := range expectedKeys {
			if value, ok := node.Labels[key]; !ok || value == "" {
				t.Errorf("Node %s missing %s label", node.Name, key)
			}
		}
	}

	if workerCount == 0 {
		t.Fatal("No worker nodes found in cluster")
	}

	logger.Infof("Successfully verified topology labels on %d worker nodes", workerCount)
	logger.Info("🎉 Topology Infrastructure test completed successfully!")
}

// Test_TAS2_MultipleCliquesWithDifferentConstraints tests PCS with multiple cliques having different topology constraints
// 1. Deploy workload with PCS (no constraint) containing 2 cliques:
//   - worker-rack: packDomain=rack (3 pods)
//   - worker-block: packDomain=block (4 pods)
//
// 2. Verify all 7 pods are scheduled successfully
// 3. Verify worker-rack pods (3) are in the same rack
// 4. Verify worker-block pods (4) are in the same block
// 5. Verify different cliques can have independent topology constraints
func Test_TAS2_MultipleCliquesWithDifferentConstraints(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
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
			Name:         "tas-indep-clq",
			YAMLPath:     "../yaml/tas-indep-clq.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS2: multiple cliques with different constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify worker-rack pods (3) are in the same rack")
	rackPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-indep-clq-0-worker-rack")
	if len(rackPods) != 3 {
		t.Fatalf("Expected 3 worker-rack pods, got %d", len(rackPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, rackPods, setup.TopologyLabelRack, logger); err != nil {
		t.Fatalf("Failed to verify worker-rack pods in same rack: %v", err)
	}

	logger.Info("4. Verify worker-block pods (4) are in the same block")
	blockPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-indep-clq-0-worker-block")
	if len(blockPods) != 4 {
		t.Fatalf("Expected 4 worker-block pods, got %d", len(blockPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, blockPods, setup.TopologyLabelBlock, logger); err != nil {
		t.Fatalf("Failed to verify worker-block pods in same block: %v", err)
	}

	logger.Info("5. Verify KAI PodGroup has correct SubGroups with topology constraints")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, podGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", podGangName, err)
	}

	// Verify top-level TopologyConstraint is empty (no PCS constraint in this test)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 standalone PCLQs - no PCSG)
	expectedSubGroups := []utils.ExpectedSubGroup{
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "worker-rack", 3, setup.TopologyLabelRack),
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "worker-block", 4, setup.TopologyLabelBlock),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS2: Multiple Cliques with Different Constraints test completed successfully!")
}

// Test_TAS3_PCSOnlyConstraint tests constraint only at PCS level with no PCSG/PCLQ constraints
// 1. Deploy workload with PCS-only constraint (packDomain: rack)
//   - PCSG: NO explicit constraint (nil)
//   - PCLQs: NO explicit constraints
//
// 2. Verify all 4 pods are in same rack (inherited from PCS)
// 3. Verify PCSG worker pods (2 total, 1 per replica)
// 4. Verify router pods (2 standalone)
// 5. Verify KAI PodGroup SubGroups: NO PCSG parent groups (because PCSG constraint is nil, per PR #357)
func Test_TAS3_PCSOnlyConstraint(t *testing.T) {
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
			Name:         "tas-sl-pcs-only",
			YAMLPath:     "../yaml/tas-sl-pcs-only.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS3: PCS-only constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all 4 pods in same rack (inherited from PCS)")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, setup.TopologyLabelRack, logger); err != nil {
		t.Fatalf("Failed to verify all pods in same rack: %v", err)
	}

	logger.Info("4. Verify PCSG worker pods (2 total, 1 per replica)")
	workerPods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "tas-sl-pcs-only-0-workers")
	if len(workerPods) != 2 {
		t.Fatalf("Expected 2 worker pods, got %d", len(workerPods))
	}

	logger.Info("5. Verify router pods (2 standalone)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-sl-pcs-only-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("6. Verify KAI PodGroup has correct SubGroups (PCS-only constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}
	basePodGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, basePodGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", basePodGangName, err)
	}

	// Verify top-level TopologyConstraint (PCS level: rack)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, setup.TopologyLabelRack, "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCLQ children + 1 router standalone = 3 total)
	// Note: PCSG parent groups are NOT created when PCSG has nil TopologyConstraint (PR #357)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// Worker PCLQs (directly under PCS constraint, no PCSG parents)
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "workers-0-worker", 1, ""),
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "workers-1-worker", 1, ""),
		// Router (standalone)
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "router", 2, ""),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS3: PCS-Only Constraint test completed successfully!")
}

// Test_TAS4_PCSGOnlyConstraint tests constraint only at PCSG level with no PCS/PCLQ constraints
// 1. Deploy workload with constraint only at PCSG level (packDomain: rack)
// 2. PCS and PCLQs have NO explicit constraints
// 3. Verify PCSG worker pods (2 total) respect rack constraint
// 4. Router pods (2 standalone) are unconstrained
func Test_TAS4_PCSGOnlyConstraint(t *testing.T) {
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
			Name:         "tas-sl-pcsg-only",
			YAMLPath:     "../yaml/tas-sl-pcsg-only.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS4: PCSG-only constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify PCSG worker pods (2 total, 1 per replica) in same rack")
	workerPods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "tas-sl-pcsg-only-0-workers")
	if len(workerPods) != 2 {
		t.Fatalf("Expected 2 worker pods, got %d", len(workerPods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, workerPods, setup.TopologyLabelRack, logger); err != nil {
		t.Fatalf("Failed to verify worker pods in same rack: %v", err)
	}

	logger.Info("4. Verify router pods (2 standalone, unconstrained)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-sl-pcsg-only-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("5. Verify KAI PodGroup has correct SubGroups (PCSG-only constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, podGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", podGangName, err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCSG parents + 2 PCLQ children + 1 router standalone = 5 total)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// PCSG replicas (parent groups, rack constraint)
		utils.CreateExpectedPCSGParentSubGroup(tc.Workload.Name, 0, "workers", 0, setup.TopologyLabelRack),
		utils.CreateExpectedPCSGParentSubGroup(tc.Workload.Name, 0, "workers", 1, setup.TopologyLabelRack),
		// Worker PCLQs (children of PCSG replicas)
		utils.CreateExpectedPCLQInPCSGSubGroup(tc.Workload.Name, 0, "workers", 0, "worker", 1, ""),
		utils.CreateExpectedPCLQInPCSGSubGroup(tc.Workload.Name, 0, "workers", 1, "worker", 1, ""),
		// Router (standalone, no constraint)
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "router", 2, ""),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS4: PCSG-Only Constraint test completed successfully!")
}

// Test_TAS5_HostLevelConstraint tests PCLQ-only constraint with host-level packing
// 1. Deploy workload with constraint only at PCLQ level (packDomain: host)
// 2. PCS has NO explicit constraint
// 3. Verify all 2 pods on same host (strictest constraint)
func Test_TAS5_HostLevelConstraint(t *testing.T) {
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
			Name:         "tas-host-level",
			YAMLPath:     "../yaml/tas-host-level.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS5: PCLQ-only host constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all pods on same host")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, setup.TopologyLabelHostname, logger); err != nil {
		t.Fatalf("Failed to verify pods on same host: %v", err)
	}

	// Additional check: verify both pods have same node name
	if len(allPods) != 2 {
		t.Fatalf("Expected 2 pods, got %d", len(allPods))
	}
	if allPods[0].Spec.NodeName != allPods[1].Spec.NodeName {
		t.Fatalf("Pods not on same node: %s vs %s", allPods[0].Spec.NodeName, allPods[1].Spec.NodeName)
	}

	logger.Info("4. Verify KAI PodGroup has correct SubGroups (PCLQ-only host constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, podGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", podGangName, err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (1 standalone PCLQ with host constraint)
	expectedSubGroups := []utils.ExpectedSubGroup{
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "worker", 2, setup.TopologyLabelHostname),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS5: Host-Level Constraint test completed successfully!")
}

// Test_TAS6_StandalonePCLQOnlyPCSZoneConstraint tests standalone PCLQ with only PCS zone constraint (no PCSG layer)
// This test differs from TAS3 in two ways:
// 1. Uses zone constraint (wider domain) instead of rack at PCS level
// 2. Has NO PCSG layer - only standalone PCLQ directly under PCS (simpler structure)
// 3. PCLQ itself has NO explicit constraint (inherits from PCS)
//
// 1. Deploy workload with PCS zone constraint and single standalone PCLQ (4 replicas)
// 2. Verify all 4 pods in same zone (PCS constraint inherited)
// 3. Verify KAI PodGroup has zone constraint at top level
// 4. Verify 1 SubGroup (standalone PCLQ) with NO additional constraint
func Test_TAS6_StandalonePCLQOnlyPCSZoneConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 4
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
			Name:         "tas-zone-level",
			YAMLPath:     "../yaml/tas-zone-level.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS6: Standalone PCLQ with only PCS zone constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all 4 pods in same zone (PCS zone constraint)")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, setup.TopologyLabelZone, logger); err != nil {
		t.Fatalf("Failed to verify pods in same zone: %v", err)
	}

	logger.Info("4. Verify KAI PodGroup has correct SubGroups (Standalone PCLQ with PCS zone constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, podGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", podGangName, err)
	}

	// Verify top-level TopologyConstraint (PCS level: zone)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, setup.TopologyLabelZone, "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (1 standalone PCLQ with NO constraint - zone is at PCS level)
	expectedSubGroups := []utils.ExpectedSubGroup{
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "worker", 4, ""),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS6: Standalone PCLQ with Only PCS Zone Constraint test completed successfully!")
}

// Test_TAS7_NoTopologyConstraint tests gang scheduling without any topology constraints
// 1. Deploy workload with no constraints at PCS, PCSG, or PCLQ levels
// 2. Verify all 4 pods scheduled (gang scheduling works)
// 3. Verify KAI PodGroup has 4 SubGroups with NO topology constraints
func Test_TAS7_NoTopologyConstraint(t *testing.T) {
	ctx := context.Background()

	logger.Info("1. Initialize a 28-node Grove cluster for topology testing")
	clientset, restConfig, dynamicClient, cleanup := prepareTestCluster(ctx, t, 28)
	defer cleanup()

	expectedPods := 4 // 2 PCSG replicas × 2 pods each
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
			Name:         "tas-no-constraint",
			YAMLPath:     "../yaml/tas-no-constraint.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (TAS7: No topology constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all 4 pods scheduled (gang scheduling works without constraints)")
	if len(allPods) != 4 {
		t.Fatalf("Expected 4 pods, got %d", len(allPods))
	}
	for _, pod := range allPods {
		if pod.Status.Phase != v1.PodRunning {
			t.Fatalf("Pod %s not running: %s", pod.Name, pod.Status.Phase)
		}
	}

	logger.Info("4. Verify KAI PodGroup has correct SubGroups (no constraints)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, tc.Workload.Name, tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGangName := fmt.Sprintf("%s-0", tc.Workload.Name)
	podGroup, err := utils.FilterPodGroupByOwner(podGroups, podGangName)
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang %s: %v", podGangName, err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCLQ children, NO constraints)
	// Note: PCSG parent groups are NOT created when PCSG has nil TopologyConstraint (PR #357)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// Worker PCLQs (directly under PCS, no PCSG parents, no constraints)
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "workers-0-worker", 2, ""),
		utils.CreateExpectedStandalonePCLQSubGroup(tc.Workload.Name, 0, "workers-1-worker", 2, ""),
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 TAS7: No Topology Constraint test completed successfully!")
}
