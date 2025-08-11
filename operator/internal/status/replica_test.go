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

package status

import (
	"testing"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestValidateReplicaAvailable(t *testing.T) {
	logger := logr.Discard()
	now := metav1.Now()

	tests := []struct {
		name          string
		resources     []*grovecorev1alpha1.PodClique
		expectedCount int
		expected      bool
	}{
		{
			name: "all resources available",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false),
				createTestPodClique("pclq-2", false, false),
			},
			expectedCount: 2,
			expected:      true,
		},
		{
			name: "some resources unavailable",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false), // available
				createTestPodClique("pclq-2", false, true),  // not available
			},
			expectedCount: 2,
			expected:      false,
		},
		{
			name: "insufficient resource count",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false),
			},
			expectedCount: 2,
			expected:      false,
		},
		{
			name: "terminating resources ignored",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false),           // available
				createTestPodCliqueTerminating("pclq-2", false, &now), // terminating, ignored
				createTestPodClique("pclq-3", false, false),           // available
			},
			expectedCount: 2,
			expected:      true,
		},
		{
			name: "insufficient after filtering terminating",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false),           // available
				createTestPodCliqueTerminating("pclq-2", false, &now), // terminating, ignored
			},
			expectedCount: 2,
			expected:      false,
		},
		{
			name:          "empty resources",
			resources:     []*grovecorev1alpha1.PodClique{},
			expectedCount: 1,
			expected:      false,
		},
		{
			name: "zero expected count",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", false, false),
			},
			expectedCount: 0,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateReplicaAvailable(
				logger,
				tt.resources,
				tt.expectedCount,
				GetPCLQCondition,
				grovecorev1alpha1.PodCliqueKind,
				0, // replica index
			)
			if result != tt.expected {
				t.Errorf("ValidateReplicaAvailable() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateReplicaScheduled(t *testing.T) {
	logger := logr.Discard()
	now := metav1.Now()

	tests := []struct {
		name          string
		resources     []*grovecorev1alpha1.PodClique
		expectedCount int
		expected      bool
	}{
		{
			name: "all resources scheduled",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", true, false),
				createTestPodClique("pclq-2", true, false),
			},
			expectedCount: 2,
			expected:      true,
		},
		{
			name: "some resources not scheduled",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", true, false),  // scheduled
				createTestPodClique("pclq-2", false, false), // not scheduled
			},
			expectedCount: 2,
			expected:      false,
		},
		{
			name: "insufficient resource count",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", true, false),
			},
			expectedCount: 2,
			expected:      false,
		},
		{
			name: "terminating resources ignored for scheduling",
			resources: []*grovecorev1alpha1.PodClique{
				createTestPodClique("pclq-1", true, false),           // scheduled
				createTestPodCliqueTerminating("pclq-2", true, &now), // terminating, ignored
				createTestPodClique("pclq-3", true, false),           // scheduled
			},
			expectedCount: 2,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateReplicaScheduled(
				logger,
				tt.resources,
				tt.expectedCount,
				GetPCLQCondition,
				grovecorev1alpha1.PodCliqueKind,
				0, // replica index
			)
			if result != tt.expected {
				t.Errorf("ValidateReplicaScheduled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestValidateReplicaState_WithPCSG(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name          string
		resources     []*grovecorev1alpha1.PodCliqueScalingGroup
		expectedCount int
		stateFunc     func([]metav1.Condition) bool
		expected      bool
	}{
		{
			name: "PCSG resources available",
			resources: []*grovecorev1alpha1.PodCliqueScalingGroup{
				createTestPCSG("pcsg-1", true, false),
				createTestPCSG("pcsg-2", true, false),
			},
			expectedCount: 2,
			stateFunc:     IsAvailable,
			expected:      true,
		},
		{
			name: "PCSG resources scheduled",
			resources: []*grovecorev1alpha1.PodCliqueScalingGroup{
				createTestPCSG("pcsg-1", true, false),
				createTestPCSG("pcsg-2", true, true), // scheduled but not available
			},
			expectedCount: 2,
			stateFunc:     IsScheduled,
			expected:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validateReplicaState(
				logger,
				tt.resources,
				tt.expectedCount,
				GetPCSGCondition,
				tt.stateFunc,
				grovecorev1alpha1.PodCliqueScalingGroupKind,
				0, // replica index
			)
			if result != tt.expected {
				t.Errorf("validateReplicaState() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// Helper functions to create test resources

func createTestPodClique(name string, scheduled, minAvailableBreached bool) *grovecorev1alpha1.PodClique {
	conditions := []metav1.Condition{}

	if scheduled {
		conditions = append(conditions, metav1.Condition{
			Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
			Status: metav1.ConditionTrue,
		})
	}

	status := metav1.ConditionFalse
	if minAvailableBreached {
		status = metav1.ConditionTrue
	}
	conditions = append(conditions, metav1.Condition{
		Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
		Status: status,
	})

	return &grovecorev1alpha1.PodClique{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: conditions,
		},
	}
}

func createTestPodCliqueTerminating(name string, scheduled bool, deletionTime *metav1.Time) *grovecorev1alpha1.PodClique {
	pclq := createTestPodClique(name, scheduled, false)
	pclq.ObjectMeta.DeletionTimestamp = deletionTime
	return pclq
}

func createTestPCSG(name string, scheduled, minAvailableBreached bool) *grovecorev1alpha1.PodCliqueScalingGroup {
	conditions := []metav1.Condition{}

	status := metav1.ConditionFalse
	if scheduled {
		status = metav1.ConditionTrue
	}
	conditions = append(conditions, metav1.Condition{
		Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
		Status: status,
	})

	status = metav1.ConditionFalse
	if minAvailableBreached {
		status = metav1.ConditionTrue
	}
	conditions = append(conditions, metav1.Condition{
		Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
		Status: status,
	})

	return &grovecorev1alpha1.PodCliqueScalingGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Status: grovecorev1alpha1.PodCliqueScalingGroupStatus{
			Conditions: conditions,
		},
	}
}
