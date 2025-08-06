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
	"context"
	"testing"
	"time"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Test constants to eliminate duplication
const (
	testNamespace = "test-ns"
	testPGSName   = "test-pgs"
	testPCSGName  = "test-pcsg"
)

func TestReconcileStatus_AvailabilityFunctionality(t *testing.T) {
	testCases := []struct {
		name              string
		pcsg              *grovecorev1alpha1.PodCliqueScalingGroup
		existingObjects   []client.Object
		expectedAvailable int32
		expectedCondition metav1.ConditionStatus
		expectedError     bool
		errorContains     string
	}{
		// Availability Computation Tests
		{
			name: "should set available replicas to 3 when all replicas are healthy",
			pcsg: createPCSGForTesting(3, nil),
			existingObjects: []client.Object{
				createPGSForTesting(),
				// Replica 0, 1, 2 - healthy PodCliques
				createPodCliqueForStatus("test-pgs-0-pcsg-pclq-0", "0"),
				createPodCliqueForStatus("test-pgs-0-pcsg-pclq-1", "1"),
				createPodCliqueForStatus("test-pgs-0-pcsg-pclq-2", "2"),
			},
			expectedAvailable: 3,
			expectedCondition: metav1.ConditionFalse,
			expectedError:     false,
		},
		{
			name: "should set available replicas to 1 when only one replica is healthy",
			pcsg: createPCSGForTesting(3, nil),
			existingObjects: []client.Object{
				createPGSForTesting(),
				createPodCliqueForStatus("test-pgs-0-clique-0", "0"),
				createPodCliqueForStatus("test-pgs-0-clique-1", "1", withMinAvailableBreached()),
				createPodCliqueForStatus("test-pgs-0-clique-2", "2", withTerminating()),
			},
			expectedAvailable: 1,
			expectedCondition: metav1.ConditionFalse, // 1 >= defaultMinAvailable (1)
			expectedError:     false,
		},
		{
			name: "should set available replicas to 0 when no replicas are healthy",
			pcsg: createPCSGForTesting(3, nil),
			existingObjects: []client.Object{
				createPGSForTesting(),
				createPodCliqueForStatus("test-pgs-0-clique-0", "0", withMinAvailableBreached()),
				createPodCliqueForStatus("test-pgs-0-clique-1", "1", withTerminating()),
				createPodCliqueForStatus("test-pgs-0-clique-2", "2", withMinAvailableBreached()),
			},
			expectedAvailable: 0,
			expectedCondition: metav1.ConditionTrue, // 0 < defaultMinAvailable (1)
			expectedError:     false,
		},
		{
			name: "should handle custom MinAvailable value of 2",
			pcsg: createPCSGForTesting(3, ptr.To[int32](2)),
			existingObjects: []client.Object{
				createPGSForTesting(),
				createPodCliqueForStatus("test-pgs-0-clique-0", "0"),
				createPodCliqueForStatus("test-pgs-0-clique-1", "1", withMinAvailableBreached()),
				createPodCliqueForStatus("test-pgs-0-clique-2", "2", withTerminating()),
			},
			expectedAvailable: 1,
			expectedCondition: metav1.ConditionTrue, // 1 < minAvailable (2)
			expectedError:     false,
		},
		{
			name: "should handle custom MinAvailable value of 0",
			pcsg: createPCSGForTesting(3, ptr.To[int32](0)),
			existingObjects: []client.Object{
				createPGSForTesting(),
				createPodCliqueForStatus("test-pgs-0-clique-0", "0", withMinAvailableBreached()),
				createPodCliqueForStatus("test-pgs-0-clique-1", "1", withTerminating()),
				createPodCliqueForStatus("test-pgs-0-clique-2", "2", withMinAvailableBreached()),
			},
			expectedAvailable: 0,
			expectedCondition: metav1.ConditionFalse, // 0 >= minAvailable (0)
			expectedError:     false,
		},

		// Error Handling Tests
		{
			name:            "should return error when PodGangSet is not found",
			pcsg:            createPCSGForTesting(3, nil),
			existingObjects: []client.Object{}, // No PGS
			expectedError:   true,
			errorContains:   "not found",
		},
		{
			name: "should return error when PCSG has no owner reference",
			pcsg: createPCSGWithoutOwnerRef(3),
			existingObjects: []client.Object{
				createPGSForTesting(),
			},
			expectedError: true,
			errorContains: "not found",
		},

		// Edge Cases
		{
			name: "should handle MinAvailable greater than replicas",
			pcsg: createPCSGForTesting(2, ptr.To[int32](5)),
			existingObjects: []client.Object{
				createPGSForTesting(),
				createPodCliqueForStatus("test-pgs-0-clique-0", "0"),
				createPodCliqueForStatus("test-pgs-0-clique-1", "1"),
			},
			expectedAvailable: 2,
			expectedCondition: metav1.ConditionTrue, // 2 < minAvailable (5)
			expectedError:     false,
		},
		{
			name: "should handle empty PodClique list",
			pcsg: createPCSGForTesting(0, nil),
			existingObjects: []client.Object{
				createPGSForTesting(),
			},
			expectedAvailable: 0,
			expectedCondition: metav1.ConditionTrue, // 0 < defaultMinAvailable (1)
			expectedError:     false,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup fake client with status subresource support
			scheme := runtime.NewScheme()
			require.NoError(t, grovecorev1alpha1.AddToScheme(scheme))

			// Add the PCSG itself to the existing objects for status updates
			allObjects := append(tc.existingObjects, tc.pcsg)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithStatusSubresource(&grovecorev1alpha1.PodCliqueScalingGroup{}).
				WithStatusSubresource(&grovecorev1alpha1.PodGangSet{}).
				WithStatusSubresource(&grovecorev1alpha1.PodClique{}).
				WithObjects(allObjects...).
				Build()

			reconciler := &Reconciler{client: fakeClient}
			logger := logr.Discard()
			ctx := context.Background()

			// Execute the function
			result := reconciler.reconcileStatus(ctx, logger, tc.pcsg)

			// Verify error expectations
			if tc.expectedError {
				assert.True(t, result.HasErrors(), "Expected error but got none")
				if tc.errorContains != "" {
					assert.Contains(t, result.GetErrors()[0].Error(), tc.errorContains)
				}
				return
			}

			// Verify no errors for success cases
			assert.False(t, result.HasErrors(), "Expected no errors but got: %v", result.GetErrors())

			// Verify availability and condition were set correctly
			assert.Equal(t, tc.expectedAvailable, tc.pcsg.Status.AvailableReplicas, "AvailableReplicas mismatch")

			// Verify MinAvailableBreached condition
			condition := getConditionByType(tc.pcsg.Status.Conditions, "MinAvailableBreached")
			require.NotNil(t, condition, "MinAvailableBreached condition should be set")
			assert.Equal(t, tc.expectedCondition, condition.Status, "MinAvailableBreached condition status mismatch")

			// Verify condition reason and message are appropriate
			if tc.expectedCondition == metav1.ConditionTrue {
				assert.Equal(t, "InsufficientReadyPCSGReplicas", condition.Reason)
				assert.Contains(t, condition.Message, "Insufficient")
			} else {
				assert.Equal(t, "SufficientReadyPCSGReplicas", condition.Reason)
				assert.Contains(t, condition.Message, "Sufficient")
			}
		})
	}
}

func TestCheckReplicaAvailability(t *testing.T) {
	testCases := []struct {
		description    string
		pclqs          []grovecorev1alpha1.PodClique
		expectedResult bool
	}{
		{
			description:    "should return true when PodClique list is empty",
			pclqs:          []grovecorev1alpha1.PodClique{},
			expectedResult: true,
		},
		{
			description: "should return true when all PodCliques are healthy",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
				createHealthyPodClique("pclq-2"),
			},
			expectedResult: true,
		},
		{
			description: "should return true when single PodClique is healthy",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
			},
			expectedResult: true,
		},
		{
			description: "should return false when single PodClique is marked for termination",
			pclqs: []grovecorev1alpha1.PodClique{
				createTerminatingPodClique("pclq-1"),
			},
			expectedResult: false,
		},
		{
			description: "should return false when one of multiple PodCliques is marked for termination",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
				createTerminatingPodClique("pclq-2"),
			},
			expectedResult: false,
		},
		{
			description: "should return false when all PodCliques are marked for termination",
			pclqs: []grovecorev1alpha1.PodClique{
				createTerminatingPodClique("pclq-1"),
				createTerminatingPodClique("pclq-2"),
			},
			expectedResult: false,
		},
		{
			description: "should return false when single PodClique has MinAvailableBreached set to true",
			pclqs: []grovecorev1alpha1.PodClique{
				createPodCliqueWithMinAvailableBreached("pclq-1", true),
			},
			expectedResult: false,
		},
		{
			description: "should return false when one of multiple PodCliques has MinAvailableBreached set to true",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
				createPodCliqueWithMinAvailableBreached("pclq-2", true),
			},
			expectedResult: false,
		},
		{
			description: "should return false when all PodCliques have MinAvailableBreached set to true",
			pclqs: []grovecorev1alpha1.PodClique{
				createPodCliqueWithMinAvailableBreached("pclq-1", true),
				createPodCliqueWithMinAvailableBreached("pclq-2", true),
			},
			expectedResult: false,
		},
		{
			description: "should return true when PodClique has MinAvailableBreached set to false explicitly",
			pclqs: []grovecorev1alpha1.PodClique{
				createPodCliqueWithMinAvailableBreached("pclq-1", false),
			},
			expectedResult: true,
		},
		{
			description: "should return false when PodClique is both terminating and has MinAvailableBreached true",
			pclqs: []grovecorev1alpha1.PodClique{
				createTerminatingPodCliqueWithMinAvailableBreached("pclq-1", true),
			},
			expectedResult: false,
		},
		{
			description: "should return true when PodClique is terminating but MinAvailableBreached is false",
			pclqs: []grovecorev1alpha1.PodClique{
				createTerminatingPodCliqueWithMinAvailableBreached("pclq-1", false),
			},
			expectedResult: false, // terminating takes precedence
		},
		{
			description: "should return false when mixed scenario with some healthy, some with MinAvailableBreached true",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
				createPodCliqueWithMinAvailableBreached("pclq-2", true),
				createHealthyPodClique("pclq-3"),
			},
			expectedResult: false,
		},
		{
			description: "should return false when mixed scenario with some healthy, some terminating",
			pclqs: []grovecorev1alpha1.PodClique{
				createHealthyPodClique("pclq-1"),
				createTerminatingPodClique("pclq-2"),
				createHealthyPodClique("pclq-3"),
			},
			expectedResult: false,
		},
		{
			description: "should return true when PodClique has no conditions set",
			pclqs: []grovecorev1alpha1.PodClique{
				createPodCliqueWithoutConditions("pclq-1"),
			},
			expectedResult: true,
		},
		{
			description: "should return true when PodClique has MinAvailableBreached with unknown status",
			pclqs: []grovecorev1alpha1.PodClique{
				createPodCliqueWithMinAvailableBreachedUnknown("pclq-1"),
			},
			expectedResult: true,
		},
	}

	t.Parallel()
	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			t.Parallel()
			logger := logr.Discard()
			result := checkReplicaAvailability(logger, "replica-0", testCase.pclqs)
			assert.Equal(t, testCase.expectedResult, result)
		})
	}
}

// Helper functions to create test PodClique objects

func createHealthyPodClique(name string) grovecorev1alpha1.PodClique {
	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: metav1.ConditionFalse,
					Reason: "SufficientReadyReplicas",
				},
			},
		},
	}
}

func createTerminatingPodClique(name string) grovecorev1alpha1.PodClique {
	now := metav1.NewTime(time.Now())
	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-ns",
			DeletionTimestamp: &now,
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: metav1.ConditionFalse,
					Reason: "SufficientReadyReplicas",
				},
			},
		},
	}
}

func createPodCliqueWithMinAvailableBreached(name string, breached bool) grovecorev1alpha1.PodClique {
	status := metav1.ConditionFalse
	reason := "SufficientReadyReplicas"
	if breached {
		status = metav1.ConditionTrue
		reason = "InsufficientReadyReplicas"
	}

	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: status,
					Reason: reason,
				},
			},
		},
	}
}

func createTerminatingPodCliqueWithMinAvailableBreached(name string, breached bool) grovecorev1alpha1.PodClique {
	now := metav1.NewTime(time.Now())
	status := metav1.ConditionFalse
	reason := "SufficientReadyReplicas"
	if breached {
		status = metav1.ConditionTrue
		reason = "InsufficientReadyReplicas"
	}

	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-ns",
			DeletionTimestamp: &now,
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: status,
					Reason: reason,
				},
			},
		},
	}
}

func createPodCliqueWithoutConditions(name string) grovecorev1alpha1.PodClique {
	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{},
		},
	}
}

func createPodCliqueWithMinAvailableBreachedUnknown(name string) grovecorev1alpha1.PodClique {
	return grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: metav1.ConditionUnknown,
					Reason: "UnknownState",
				},
			},
		},
	}
}

// Helper functions for reconcileStatus availability tests

// Option pattern for PodClique creation
type podCliqueOption func(*grovecorev1alpha1.PodClique)

func withMinAvailableBreached() podCliqueOption {
	return func(pclq *grovecorev1alpha1.PodClique) {
		pclq.Status.Conditions = []metav1.Condition{
			{
				Type:   "MinAvailableBreached",
				Status: metav1.ConditionTrue,
				Reason: "InsufficientReadyReplicas",
			},
		}
	}
}

func withTerminating() podCliqueOption {
	return func(pclq *grovecorev1alpha1.PodClique) {
		now := metav1.NewTime(time.Now())
		pclq.DeletionTimestamp = &now
		pclq.Finalizers = []string{"test-finalizer"}
	}
}

// Single PodClique builder function - replaces all individual helper functions
func createPodCliqueForStatus(name, replicaIndex string, opts ...podCliqueOption) *grovecorev1alpha1.PodClique {
	pclq := &grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				grovecorev1alpha1.LabelManagedByKey:                      grovecorev1alpha1.LabelManagedByValue,
				grovecorev1alpha1.LabelPartOfKey:                         testPGSName,
				grovecorev1alpha1.LabelPodCliqueScalingGroup:             testPCSGName,
				grovecorev1alpha1.LabelComponentKey:                      common.NamePCSGPodClique,
				grovecorev1alpha1.LabelPodGangSetReplicaIndex:            "0",
				grovecorev1alpha1.LabelPodCliqueScalingGroupReplicaIndex: replicaIndex,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "PodCliqueScalingGroup",
					Name: testPCSGName,
				},
			},
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: []metav1.Condition{
				{
					Type:   "MinAvailableBreached",
					Status: metav1.ConditionFalse,
					Reason: "SufficientReadyReplicas",
				},
			},
		},
	}

	// Apply options
	for _, opt := range opts {
		opt(pclq)
	}

	return pclq
}

func createPCSGForTesting(replicas int32, minAvailable *int32) *grovecorev1alpha1.PodCliqueScalingGroup {
	pcsg := &grovecorev1alpha1.PodCliqueScalingGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPCSGName,
			Namespace: testNamespace,
			Labels: map[string]string{
				grovecorev1alpha1.LabelPartOfKey:              testPGSName,
				grovecorev1alpha1.LabelPodGangSetReplicaIndex: "0",
			},
		},
		Spec: grovecorev1alpha1.PodCliqueScalingGroupSpec{
			Replicas:     replicas,
			MinAvailable: minAvailable,
		},
		Status: grovecorev1alpha1.PodCliqueScalingGroupStatus{
			Conditions: []metav1.Condition{},
		},
	}
	return pcsg
}

func createPCSGWithoutOwnerRef(replicas int32) *grovecorev1alpha1.PodCliqueScalingGroup {
	pcsg := &grovecorev1alpha1.PodCliqueScalingGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPCSGName,
			Namespace: testNamespace,
		},
		Spec: grovecorev1alpha1.PodCliqueScalingGroupSpec{
			Replicas: replicas,
		},
		Status: grovecorev1alpha1.PodCliqueScalingGroupStatus{
			Conditions: []metav1.Condition{},
		},
	}
	return pcsg
}

func createPGSForTesting() *grovecorev1alpha1.PodGangSet {
	return &grovecorev1alpha1.PodGangSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPGSName,
			Namespace: testNamespace,
		},
		Spec: grovecorev1alpha1.PodGangSetSpec{
			Replicas: 1,
		},
	}
}

func getConditionByType(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
