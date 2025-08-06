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

package podcliquescalinggroup

import (
	"testing"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/testutils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "test-ns"
	testPGSName   = "test-pgs"
	testPCSGName  = "test-pcsg"
)

// Simple test helpers following KISS principles

func buildTestPCSG(replicas int32, minAvailable *int32) *grovecorev1alpha1.PodCliqueScalingGroup {
	builder := testutils.NewPCSGBuilder(testPCSGName, testNamespace, testPGSName, 0).
		WithReplicas(replicas)

	if minAvailable != nil {
		builder = builder.WithMinAvailable(*minAvailable)
	}

	pcsg := builder.Build()
	// Set CliqueNames directly since there's no builder method
	pcsg.Spec.CliqueNames = []string{"frontend", "backend"}
	return pcsg
}

func buildHealthyCliques(count int) []grovecorev1alpha1.PodClique {
	cliques := make([]grovecorev1alpha1.PodClique, count)
	for i := 0; i < count; i++ {
		cliques[i] = *testutils.NewPodCliqueBuilder("test-pgs-0-clique", testNamespace, testPGSName, i).
			WithOptions(testutils.WithPCLQAvailable()).
			Build()
	}
	return cliques
}

func buildBreachedCliques(count int) []grovecorev1alpha1.PodClique {
	cliques := make([]grovecorev1alpha1.PodClique, count)
	for i := 0; i < count; i++ {
		cliques[i] = *testutils.NewPodCliqueBuilder("test-pgs-0-clique", testNamespace, testPGSName, i).
			WithOptions(testutils.WithPCLQMinAvailableBreached()).
			Build()
	}
	return cliques
}

func buildTerminatingCliques(count int) []grovecorev1alpha1.PodClique {
	cliques := make([]grovecorev1alpha1.PodClique, count)
	for i := 0; i < count; i++ {
		cliques[i] = *testutils.NewPodCliqueBuilder("test-pgs-0-clique", testNamespace, testPGSName, i).
			WithOptions(testutils.WithPCLQTerminating()).
			Build()
	}
	return cliques
}

func assertCondition(t *testing.T, pcsg *grovecorev1alpha1.PodCliqueScalingGroup, expectedBreached bool) {
	condition := findCondition(pcsg.Status.Conditions, grovecorev1alpha1.ConditionTypeMinAvailableBreached)
	require.NotNil(t, condition, "MinAvailableBreached condition should exist")

	isBreached := condition.Status == metav1.ConditionTrue
	assert.Equal(t, expectedBreached, isBreached, "MinAvailableBreached condition mismatch")
}

func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}

// Unit tests for checkReplicaAvailability function

func TestCheckReplicaAvailability(t *testing.T) {
	logger := testutils.SetupTestLogger()
	pcsg := buildTestPCSG(1, nil)

	tests := []struct {
		name     string
		cliques  []grovecorev1alpha1.PodClique
		expected bool
	}{
		{
			name:     "healthy cliques",
			cliques:  buildHealthyCliques(2),
			expected: true,
		},
		{
			name:     "breached cliques",
			cliques:  buildBreachedCliques(2),
			expected: false,
		},
		{
			name:     "terminating cliques",
			cliques:  buildTerminatingCliques(2),
			expected: false,
		},
		{
			name:     "mixed healthy and breached",
			cliques:  append(buildHealthyCliques(1), buildBreachedCliques(1)...),
			expected: false,
		},
		{
			name:     "wrong clique count - too few",
			cliques:  buildHealthyCliques(1), // pcsg expects 2 cliques
			expected: false,
		},
		{
			name:     "wrong clique count - too many",
			cliques:  buildHealthyCliques(3), // pcsg expects 2 cliques
			expected: false,
		},
		{
			name:     "empty cliques",
			cliques:  []grovecorev1alpha1.PodClique{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkReplicaAvailability(logger, "replica-0", tt.cliques, pcsg)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Unit tests for computeMinAvailableBreachedCondition function

func TestComputeMinAvailableBreachedCondition(t *testing.T) {
	reconciler := &Reconciler{}

	tests := []struct {
		name              string
		availableReplicas int32
		minAvailable      *int32
		expectedBreached  bool
		expectedMessage   string
	}{
		{
			name:              "default minAvailable - sufficient",
			availableReplicas: 1,
			minAvailable:      nil,
			expectedBreached:  false,
			expectedMessage:   "Sufficient PodCliqueScalingGroup replicas, expected at least: 1, found: 1",
		},
		{
			name:              "default minAvailable - insufficient",
			availableReplicas: 0,
			minAvailable:      nil,
			expectedBreached:  true,
			expectedMessage:   "Insufficient PodCliqueScalingGroup replicas, expected at least: 1, found: 0",
		},
		{
			name:              "custom minAvailable - sufficient",
			availableReplicas: 3,
			minAvailable:      ptr.To(int32(2)),
			expectedBreached:  false,
			expectedMessage:   "Sufficient PodCliqueScalingGroup replicas, expected at least: 1, found: 3",
		},
		{
			name:              "custom minAvailable - insufficient",
			availableReplicas: 1,
			minAvailable:      ptr.To(int32(3)),
			expectedBreached:  true,
			expectedMessage:   "Insufficient PodCliqueScalingGroup replicas, expected at least: 1, found: 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pcsg := buildTestPCSG(3, tt.minAvailable)

			condition := reconciler.computeMinAvailableBreachedCondition(tt.availableReplicas, pcsg)

			isBreached := condition.Status == metav1.ConditionTrue
			assert.Equal(t, tt.expectedBreached, isBreached, "condition status mismatch")
			assert.Equal(t, grovecorev1alpha1.ConditionTypeMinAvailableBreached, condition.Type)
			assert.Contains(t, condition.Message, tt.expectedMessage)

			if tt.expectedBreached {
				assert.Equal(t, grovecorev1alpha1.ConditionReasonInsufficientReadyPCSGReplicas, condition.Reason)
			} else {
				assert.Equal(t, grovecorev1alpha1.ConditionReasonSufficientReadyPCSGReplicas, condition.Reason)
			}
		})
	}
}

// Test computePCSGAvailability function directly

func TestComputePCSGAvailability(t *testing.T) {
	pcsg := buildTestPCSG(1, nil)

	// Create PodCliques with proper owner references
	frontend := testutils.NewPCSGPodCliqueBuilder("frontend-0", testNamespace, testPGSName, testPCSGName, 0, 0).
		WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
		WithOptions(testutils.WithPCLQAvailable()).Build()
	backend := testutils.NewPCSGPodCliqueBuilder("backend-0", testNamespace, testPGSName, testPCSGName, 0, 0).
		WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
		WithOptions(testutils.WithPCLQAvailable()).Build()

	healthyCliques := []client.Object{frontend, backend}

	// Setup fake client
	fakeClient := testutils.SetupFakeClient(healthyCliques...)
	reconciler := &Reconciler{client: fakeClient}

	// Execute computePCSGAvailability directly
	available, err := reconciler.computePCSGAvailability(
		testutils.SetupTestContext(),
		testutils.SetupTestLogger(),
		testPGSName,
		pcsg,
	)

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, int32(1), available, "should find 1 available replica")
}

// Simple integration test for basic scenarios

func TestReconcileStatus_Basic(t *testing.T) {
	// Test simple scenario: 1 replica with 2 healthy cliques
	pcsg := buildTestPCSG(1, nil)
	pgs := testutils.NewPGSBuilder(testPGSName, testNamespace).Build()

	// Create 2 healthy cliques for replica 0
	healthyCliques := []client.Object{
		testutils.NewPCSGPodCliqueBuilder("frontend-0", testNamespace, testPGSName, testPCSGName, 0, 0).
			WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
			WithOptions(testutils.WithPCLQAvailable()).Build(),
		testutils.NewPCSGPodCliqueBuilder("backend-0", testNamespace, testPGSName, testPCSGName, 0, 0).
			WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
			WithOptions(testutils.WithPCLQAvailable()).Build(),
	}

	// Setup fake client
	allResources := append([]client.Object{pcsg, pgs}, healthyCliques...)
	fakeClient := testutils.SetupFakeClient(allResources...)
	reconciler := &Reconciler{client: fakeClient}

	// Execute reconciliation
	result := reconciler.reconcileStatus(
		testutils.SetupTestContext(),
		testutils.SetupTestLogger(),
		pcsg,
	)

	// Verify results
	require.False(t, result.HasErrors(), "reconcileStatus should not return errors")
	assert.Equal(t, int32(1), pcsg.Status.AvailableReplicas, "should have 1 available replica")
	assertCondition(t, pcsg, false)
}

// Full integration test for reconcileStatus function

func TestReconcileStatus(t *testing.T) {
	tests := []struct {
		name               string
		pcsgReplicas       int32
		minAvailable       *int32
		healthyCliques     int
		breachedCliques    int
		terminatingCliques int
		expectedAvailable  int32
		expectedBreached   bool
	}{
		{
			name:              "single healthy replica",
			pcsgReplicas:      1,
			healthyCliques:    2, // 1 replica * 2 cliques
			expectedAvailable: 1,
			expectedBreached:  false,
		},
		{
			name:              "some breached",
			pcsgReplicas:      2,
			healthyCliques:    2, // replica 0: healthy cliques
			breachedCliques:   2, // replica 1: breached cliques
			expectedAvailable: 1,
			expectedBreached:  false, // 1 >= default(1)
		},
		{
			name:              "all breached",
			pcsgReplicas:      1,
			breachedCliques:   2,
			expectedAvailable: 0,
			expectedBreached:  true, // 0 < default(1)
		},
		{
			name:              "custom minAvailable breached",
			pcsgReplicas:      2,
			minAvailable:      ptr.To(int32(2)),
			healthyCliques:    2, // only 1 replica healthy
			breachedCliques:   2,
			expectedAvailable: 1,
			expectedBreached:  true, // 1 < 2
		},
		{
			name:               "terminating cliques",
			pcsgReplicas:       1,
			terminatingCliques: 2,
			expectedAvailable:  0,
			expectedBreached:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup PCSG and PGS
			pcsg := buildTestPCSG(tt.pcsgReplicas, tt.minAvailable)
			pgs := testutils.NewPGSBuilder(testPGSName, testNamespace).Build()

			// Build cliques for each replica
			var allCliques []client.Object
			replicaIndex := 0

			// Add healthy cliques
			for i := 0; i < tt.healthyCliques; i += 2 {
				frontendName := "test-pgs-0-frontend-" + string(rune('0'+replicaIndex))
				backendName := "test-pgs-0-backend-" + string(rune('0'+replicaIndex))
				allCliques = append(allCliques,
					testutils.NewPCSGPodCliqueBuilder(frontendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQAvailable()).Build(),
					testutils.NewPCSGPodCliqueBuilder(backendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQAvailable()).Build(),
				)
				replicaIndex++
			}

			// Add breached cliques
			for i := 0; i < tt.breachedCliques; i += 2 {
				frontendName := "test-pgs-0-frontend-" + string(rune('0'+replicaIndex))
				backendName := "test-pgs-0-backend-" + string(rune('0'+replicaIndex))
				allCliques = append(allCliques,
					testutils.NewPCSGPodCliqueBuilder(frontendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQMinAvailableBreached()).Build(),
					testutils.NewPCSGPodCliqueBuilder(backendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQMinAvailableBreached()).Build(),
				)
				replicaIndex++
			}

			// Add terminating cliques
			for i := 0; i < tt.terminatingCliques; i += 2 {
				frontendName := "test-pgs-0-frontend-" + string(rune('0'+replicaIndex))
				backendName := "test-pgs-0-backend-" + string(rune('0'+replicaIndex))
				allCliques = append(allCliques,
					testutils.NewPCSGPodCliqueBuilder(frontendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQTerminating()).Build(),
					testutils.NewPCSGPodCliqueBuilder(backendName, testNamespace, testPGSName, testPCSGName, 0, replicaIndex).
						WithOwnerReference("PodCliqueScalingGroup", testPCSGName, "").
						WithOptions(testutils.WithPCLQTerminating()).Build(),
				)
				replicaIndex++
			}

			// Create fake client with all resources
			allResources := append([]client.Object{pcsg, pgs}, allCliques...)
			fakeClient := testutils.SetupFakeClient(allResources...)
			reconciler := &Reconciler{client: fakeClient}

			// Execute reconciliation
			result := reconciler.reconcileStatus(
				testutils.SetupTestContext(),
				testutils.SetupTestLogger(),
				pcsg,
			)

			// Verify results
			require.False(t, result.HasErrors(), "reconcileStatus should not return errors")
			assert.Equal(t, tt.expectedAvailable, pcsg.Status.AvailableReplicas, "available replicas mismatch")
			assert.Equal(t, tt.pcsgReplicas, pcsg.Status.Replicas, "total replicas should be set")

			assertCondition(t, pcsg, tt.expectedBreached)
		})
	}
}
