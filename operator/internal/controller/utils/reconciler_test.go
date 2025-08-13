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

package utils

import (
	"errors"
	"testing"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	groveerr "github.com/NVIDIA/grove/operator/internal/errors"

	"github.com/stretchr/testify/assert"
)

// Test helper functions
func newGroveError(code grovecorev1alpha1.ErrorCode, message string) error {
	return &groveerr.GroveError{
		Code:    code,
		Message: message,
	}
}

func TestShouldRequeueAfter(t *testing.T) {
	testCases := []struct {
		description string
		err         error
		expected    bool
	}{
		{
			description: "should return true for GroveError with RequeueAfter code",
			err:         newGroveError(groveerr.ErrCodeRequeueAfter, "test requeue after error"),
			expected:    true,
		},
		{
			description: "should return false for GroveError with ContinueReconcileAndRequeue code",
			err:         newGroveError(groveerr.ErrCodeContinueReconcileAndRequeue, "test continue and requeue error"),
			expected:    false,
		},
		{
			description: "should return false for GroveError with other code",
			err:         newGroveError("ERR_OTHER_CODE", "test other error"),
			expected:    false,
		},
		{
			description: "should return false for non-GroveError",
			err:         errors.New("standard error"),
			expected:    false,
		},
		{
			description: "should return false for nil error",
			err:         nil,
			expected:    false,
		},
		{
			description: "should return false for wrapped standard error",
			err:         errors.New("wrapped: standard error"),
			expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := ShouldRequeueAfter(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestShouldContinueReconcileAndRequeue(t *testing.T) {
	testCases := []struct {
		description string
		err         error
		expected    bool
	}{
		{
			description: "should return true for GroveError with ContinueReconcileAndRequeue code",
			err:         newGroveError(groveerr.ErrCodeContinueReconcileAndRequeue, "test continue and requeue error"),
			expected:    true,
		},
		{
			description: "should return false for GroveError with RequeueAfter code",
			err:         newGroveError(groveerr.ErrCodeRequeueAfter, "test requeue after error"),
			expected:    false,
		},
		{
			description: "should return false for GroveError with other code",
			err:         newGroveError("ERR_OTHER_CODE", "test other error"),
			expected:    false,
		},
		{
			description: "should return false for non-GroveError",
			err:         errors.New("standard error"),
			expected:    false,
		},
		{
			description: "should return false for nil error",
			err:         nil,
			expected:    false,
		},
		{
			description: "should return false for wrapped standard error",
			err:         errors.New("wrapped: standard error"),
			expected:    false,
		},
		{
			description: "should return false for GroveError with unknown code",
			err:         newGroveError("unknown-code", "test unknown code error"),
			expected:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := ShouldContinueReconcileAndRequeue(tc.err)
			assert.Equal(t, tc.expected, result)
		})
	}
}
