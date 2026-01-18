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
	"k8s.io/utils/ptr"
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
			Name:         "tas-indep-clq",
			YAMLPath:     "../yaml/tas-indep-clq.yaml",
			Namespace:    "default",
			ExpectedPods: expectedPods,
		},
	}

	logger.Info("2. Deploy workload (BP-1: multiple cliques with different constraints)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify worker-rack pods (3) are in the same rack")
	rackPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-indep-clq-0-worker-rack")
	if len(rackPods) != 3 {
		t.Fatalf("Expected 3 worker-rack pods, got %d", len(rackPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, rackPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify worker-rack pods in same rack: %v", err)
	}

	logger.Info("4. Verify worker-block pods (4) are in the same block")
	blockPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-indep-clq-0-worker-block")
	if len(blockPods) != 4 {
		t.Fatalf("Expected 4 worker-block pods, got %d", len(blockPods))
	}

	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, blockPods, "kubernetes.io/block", logger); err != nil {
		t.Fatalf("Failed to verify worker-block pods in same block: %v", err)
	}

	logger.Info("5. Verify KAI PodGroup has correct SubGroups with topology constraints")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-indep-clq", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-indep-clq-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-indep-clq-0: %v", err)
	}

	// Verify top-level TopologyConstraint is empty (no PCS constraint in this test)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 standalone PCLQs - no PCSG)
	expectedSubGroups := []utils.ExpectedSubGroup{
		{
			Name:                  "tas-indep-clq-0-worker-rack",
			MinMember:             3,
			Parent:                nil,
			RequiredTopologyLevel: "kubernetes.io/rack",
		},
		{
			Name:                  "tas-indep-clq-0-worker-block",
			MinMember:             4,
			Parent:                nil,
			RequiredTopologyLevel: "kubernetes.io/block",
		},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
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
			Name:         "tas-sl-pcs-only",
			YAMLPath:     "../yaml/tas-sl-pcs-only.yaml",
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
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-sl-pcs-only", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-sl-pcs-only-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-sl-pcs-only-0: %v", err)
	}

	// Verify top-level TopologyConstraint (PCS level: rack)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "kubernetes.io/rack", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCSG parents + 2 PCLQ children + 1 router standalone = 5 total)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// PCSG replicas (parent groups, no explicit constraint)
		{Name: "tas-sl-pcs-only-0-workers-0", MinMember: 0, Parent: nil},
		{Name: "tas-sl-pcs-only-0-workers-1", MinMember: 0, Parent: nil},
		// Worker PCLQs (children of PCSG replicas)
		{Name: "tas-sl-pcs-only-0-workers-0-worker", MinMember: 1, Parent: ptr.To("tas-sl-pcs-only-0-workers-0")},
		{Name: "tas-sl-pcs-only-0-workers-1-worker", MinMember: 1, Parent: ptr.To("tas-sl-pcs-only-0-workers-1")},
		// Router (standalone)
		{Name: "tas-sl-pcs-only-0-router", MinMember: 2, Parent: nil},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
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
			Name:         "tas-sl-pcsg-only",
			YAMLPath:     "../yaml/tas-sl-pcsg-only.yaml",
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
	workerPods := utils.FilterPodsByLabel(allPods, "grove.io/podcliquescalinggroup", "tas-sl-pcsg-only-0-workers")
	if len(workerPods) != 2 {
		t.Fatalf("Expected 2 worker pod, got %d", len(workerPods))
	}
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, workerPods, "kubernetes.io/rack", logger); err != nil {
		t.Fatalf("Failed to verify worker pods in same rack: %v", err)
	}

	logger.Info("4. Verify router pods (2 standalone, unconstrained)")
	routerPods := utils.FilterPodsByLabel(allPods, "grove.io/podclique", "tas-sl-pcsg-only-0-router")
	if len(routerPods) != 2 {
		t.Fatalf("Expected 2 router pods, got %d", len(routerPods))
	}

	logger.Info("5. Verify KAI PodGroup has correct SubGroups (PCSG-only constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-sl-pcsg-only", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-sl-pcsg-only-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-sl-pcsg-only-0: %v", err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCSG parents + 2 PCLQ children + 1 router standalone = 5 total)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// PCSG replicas (parent groups, rack constraint)
		{Name: "tas-sl-pcsg-only-0-workers-0", MinMember: 0, Parent: nil, RequiredTopologyLevel: "kubernetes.io/rack"},
		{Name: "tas-sl-pcsg-only-0-workers-1", MinMember: 0, Parent: nil, RequiredTopologyLevel: "kubernetes.io/rack"},
		// Worker PCLQs (children of PCSG replicas)
		{Name: "tas-sl-pcsg-only-0-workers-0-worker", MinMember: 1, Parent: ptr.To("tas-sl-pcsg-only-0-workers-0")},
		{Name: "tas-sl-pcsg-only-0-workers-1-worker", MinMember: 1, Parent: ptr.To("tas-sl-pcsg-only-0-workers-1")},
		// Router (standalone, no constraint)
		{Name: "tas-sl-pcsg-only-0-router", MinMember: 2, Parent: nil},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
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
			Name:         "tas-host-level",
			YAMLPath:     "../yaml/tas-host-level.yaml",
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

	logger.Info("4. Verify KAI PodGroup has correct SubGroups (PCLQ-only host constraint)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-host-level", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-host-level-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-host-level-0: %v", err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (1 standalone PCLQ with host constraint)
	expectedSubGroups := []utils.ExpectedSubGroup{
		{Name: "tas-host-level-0-worker", MinMember: 2, Parent: nil, RequiredTopologyLevel: "kubernetes.io/hostname"},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 PC-1: Host-Level Constraint test completed successfully!")
}

// Test_TAS_SP2_PCSPlusPCLQConstraint tests PCS with block constraint and standalone PCLQ with host constraint
// Scenario SP-2:
// 1. Deploy workload with PCS block constraint and PCLQ host constraint (no PCSG layer)
// 2. Verify 2 pods on same host (PCLQ constraint, strictest)
// 3. Verify both pods in same block (PCS constraint inherited)
// 4. Verify KAI PodGroup has block constraint at top level, 1 SubGroup with host constraint
func Test_TAS_ZL1_ZoneLevelConstraint(t *testing.T) {
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

	logger.Info("2. Deploy workload (ZL-1: PCS zone constraint)")
	allPods, err := deployWorkloadAndGetPods(tc, expectedPods)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	logger.Info("3. Verify all 4 pods in same zone (PCS zone constraint)")
	if err := utils.VerifyPodsInSameTopologyDomain(tc.Ctx, tc.Clientset, allPods, "kubernetes.io/zone", logger); err != nil {
		t.Fatalf("Failed to verify pods in same zone: %v", err)
	}

	logger.Info("4. Verify KAI PodGroup has correct SubGroups (zone at PCS level)")
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-zone-level", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-zone-level-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-zone-level-0: %v", err)
	}

	// Verify top-level TopologyConstraint (PCS level: zone)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "kubernetes.io/zone", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (1 standalone PCLQ with NO constraint - zone is at PCS level)
	expectedSubGroups := []utils.ExpectedSubGroup{
		{Name: "tas-zone-level-0-worker", MinMember: 4, Parent: nil},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 ZL-1: Zone-Level Constraint test completed successfully!")
}

// Test_TAS_SL3_NoTopologyConstraint tests gang scheduling without any topology constraints
// Scenario SL-3:
// 1. Deploy workload with no constraints at PCS, PCSG, or PCLQ levels
// 2. Verify all 4 pods scheduled (gang scheduling works)
// 3. Verify KAI PodGroup has 4 SubGroups with NO topology constraints
func Test_TAS_SL3_NoTopologyConstraint(t *testing.T) {
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

	logger.Info("2. Deploy workload (SL-3: No topology constraints)")
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
	podGroups, err := utils.WaitForKAIPodGroups(tc.Ctx, tc.DynamicClient, tc.Namespace, "tas-no-constraint", tc.Timeout, tc.Interval, logger)
	if err != nil {
		t.Fatalf("Failed to get KAI PodGroups: %v", err)
	}

	podGroup, err := utils.FilterPodGroupByOwner(podGroups, "tas-no-constraint-0")
	if err != nil {
		t.Fatalf("Failed to find PodGroup for PodGang tas-no-constraint-0: %v", err)
	}

	// Verify top-level TopologyConstraint (no PCS constraint)
	if err := utils.VerifyKAIPodGroupTopologyConstraint(podGroup, "", "", logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup top-level constraint: %v", err)
	}

	// Verify SubGroups (2 PCSG parents + 2 PCLQ children, all with NO constraints)
	expectedSubGroups := []utils.ExpectedSubGroup{
		// PCSG replicas (parent groups, no constraint)
		{Name: "tas-no-constraint-0-workers-0", MinMember: 0, Parent: nil},
		{Name: "tas-no-constraint-0-workers-1", MinMember: 0, Parent: nil},
		// Worker PCLQs (children, no constraint)
		{Name: "tas-no-constraint-0-workers-0-worker", MinMember: 2, Parent: ptr.To("tas-no-constraint-0-workers-0")},
		{Name: "tas-no-constraint-0-workers-1-worker", MinMember: 2, Parent: ptr.To("tas-no-constraint-0-workers-1")},
	}
	if err := utils.VerifyKAIPodGroupSubGroups(podGroup, expectedSubGroups, logger); err != nil {
		t.Fatalf("Failed to verify KAI PodGroup SubGroups: %v", err)
	}

	logger.Info("🎉 SL-3: No Topology Constraint test completed successfully!")
}
