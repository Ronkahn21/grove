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

package validation

import (
	"testing"

	configv1alpha1 "github.com/ai-dynamo/grove/operator/api/config/v1alpha1"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateTopologyConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		config      configv1alpha1.TopologyConfiguration
		expectError bool
		errorField  string
	}{
		{
			name: "valid: enabled with name",
			config: configv1alpha1.TopologyConfiguration{
				Enabled: true,
				Name:    "my-topology",
			},
			expectError: false,
		},
		{
			name: "valid: disabled with no name",
			config: configv1alpha1.TopologyConfiguration{
				Enabled: false,
				Name:    "",
			},
			expectError: false,
		},
		{
			name: "valid: disabled with name",
			config: configv1alpha1.TopologyConfiguration{
				Enabled: false,
				Name:    "my-topology",
			},
			expectError: false,
		},
		{
			name: "invalid: enabled with empty name",
			config: configv1alpha1.TopologyConfiguration{
				Enabled: true,
				Name:    "",
			},
			expectError: true,
			errorField:  "topology.name",
		},
		{
			name: "invalid: enabled with whitespace-only name",
			config: configv1alpha1.TopologyConfiguration{
				Enabled: true,
				Name:    "   ",
			},
			expectError: true,
			errorField:  "topology.name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateTopologyConfiguration(tt.config, field.NewPath("topology"))

			if tt.expectError {
				assert.NotEmpty(t, errs, "expected validation errors but got none")
				if len(errs) > 0 {
					assert.Equal(t, tt.errorField, errs[0].Field)
				}
			} else {
				assert.Empty(t, errs, "expected no validation errors but got: %v", errs)
			}
		})
	}
}
