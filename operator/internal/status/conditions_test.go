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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsScheduled(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		expected   bool
	}{
		{
			name: "scheduled when PodCliqueScheduled is True",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionTrue,
				},
			},
			expected: true,
		},
		{
			name: "not scheduled when PodCliqueScheduled is False",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionFalse,
				},
			},
			expected: false,
		},
		{
			name: "not scheduled when PodCliqueScheduled is Unknown",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionUnknown,
				},
			},
			expected: false,
		},
		{
			name:       "not scheduled when condition is missing",
			conditions: []metav1.Condition{},
			expected:   false,
		},
		{
			name: "not scheduled when different condition is present",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionTrue,
				},
			},
			expected: false,
		},
		{
			name: "scheduled ignores other conditions",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionTrue,
				},
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionTrue,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsScheduled(tt.conditions)
			if result != tt.expected {
				t.Errorf("IsScheduled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsAvailable(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		expected   bool
	}{
		{
			name: "available when MinAvailableBreached is False",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionFalse,
				},
			},
			expected: true,
		},
		{
			name: "not available when MinAvailableBreached is True",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionTrue,
				},
			},
			expected: false,
		},
		{
			name: "not available when MinAvailableBreached is Unknown",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionUnknown,
				},
			},
			expected: false,
		},
		{
			name:       "not available when condition is missing",
			conditions: []metav1.Condition{},
			expected:   false,
		},
		{
			name: "not available when different condition is present",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionFalse,
				},
			},
			expected: false,
		},
		{
			name: "available ignores other conditions",
			conditions: []metav1.Condition{
				{
					Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
					Status: metav1.ConditionFalse,
				},
				{
					Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
					Status: metav1.ConditionFalse,
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAvailable(tt.conditions)
			if result != tt.expected {
				t.Errorf("IsAvailable() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetPCLQCondition(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
			Status: metav1.ConditionFalse,
		},
	}

	pclq := &grovecorev1alpha1.PodClique{
		Status: grovecorev1alpha1.PodCliqueStatus{
			Conditions: conditions,
		},
	}

	result := GetPCLQCondition(pclq)
	if len(result) != len(conditions) {
		t.Errorf("GetPCLQCondition() returned %d conditions, want %d", len(result), len(conditions))
	}

	if result[0].Type != conditions[0].Type {
		t.Errorf("GetPCLQCondition() returned condition type %s, want %s", result[0].Type, conditions[0].Type)
	}
}

func TestGetPCSGCondition(t *testing.T) {
	conditions := []metav1.Condition{
		{
			Type:   grovecorev1alpha1.ConditionTypePodCliqueScheduled,
			Status: metav1.ConditionTrue,
		},
		{
			Type:   grovecorev1alpha1.ConditionTypeMinAvailableBreached,
			Status: metav1.ConditionFalse,
		},
	}

	pcsg := &grovecorev1alpha1.PodCliqueScalingGroup{
		Status: grovecorev1alpha1.PodCliqueScalingGroupStatus{
			Conditions: conditions,
		},
	}

	result := GetPCSGCondition(pcsg)
	if len(result) != len(conditions) {
		t.Errorf("GetPCSGCondition() returned %d conditions, want %d", len(result), len(conditions))
	}

	if result[0].Type != conditions[0].Type {
		t.Errorf("GetPCSGCondition() returned first condition type %s, want %s", result[0].Type, conditions[0].Type)
	}

	if result[1].Type != conditions[1].Type {
		t.Errorf("GetPCSGCondition() returned second condition type %s, want %s", result[1].Type, conditions[1].Type)
	}
}
