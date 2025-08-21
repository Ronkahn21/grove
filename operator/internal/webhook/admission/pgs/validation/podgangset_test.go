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
	"errors"
	"fmt"
	"testing"
	"time"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	testutils "github.com/NVIDIA/grove/operator/test/utils"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
)

// Temporary helper function for remaining tests - to be refactored
func createDummyPodGangSet(name string) *grovecorev1alpha1.PodGangSet {
	return testutils.NewPodGangSetBuilder(name, "default").
		WithReplicas(1).
		WithTerminationDelay(30 * time.Second).
		WithCliqueStartupType(ptr.To(grovecorev1alpha1.CliqueStartupTypeAnyOrder)).
		WithPodCliqueTemplateSpec(
			testutils.NewPodCliqueTemplateSpecBuilder("test").
				WithReplicas(1).
				WithRoleName("dummy-role").
				WithMinAvailable(1).
				Build()).
		Build()
}

func createDummyPodCliqueTemplate(name string) *grovecorev1alpha1.PodCliqueTemplateSpec {
	return testutils.NewPodCliqueTemplateSpecBuilder(name).
		WithReplicas(1).
		WithRoleName(fmt.Sprintf("dummy-%s-role", name)).
		WithMinAvailable(1).
		Build()
}

func createScalingGroupConfig(name string, cliqueNames []string) grovecorev1alpha1.PodCliqueScalingGroupConfig {
	return grovecorev1alpha1.PodCliqueScalingGroupConfig{
		Name:        name,
		CliqueNames: cliqueNames,
	}
}

func TestResourceNamingValidation(t *testing.T) {
	testCases := []struct {
		description      string
		pgsName          string
		cliqueNames      []string
		scalingGroups    []grovecorev1alpha1.PodCliqueScalingGroupConfig
		expectError      bool
		expectedErrMsg   string
		expectedErrCount int
	}{
		{
			description: "Valid resource names",
			pgsName:     "inference",
			cliqueNames: []string{"prefill", "decode"},
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				createScalingGroupConfig("workers", []string{"prefill", "decode"}),
			},
			expectError: false,
		},
		{
			description:      "PodClique template name exceeds character limit",
			pgsName:          "verylongpodgangsetnamethatisverylong",
			cliqueNames:      []string{"verylongpodcliquenamethatexceedslimit"},
			expectError:      true,
			expectedErrMsg:   "combined resource name length",
			expectedErrCount: 2,
		},
		{
			description:    "Empty PodClique template name",
			pgsName:        "inference",
			cliqueNames:    []string{""},
			expectError:    true,
			expectedErrMsg: "field cannot be empty",
		},
		{
			description:    "PodClique template name with invalid characters",
			pgsName:        "inference",
			cliqueNames:    []string{"prefill_worker"},
			expectError:    true,
			expectedErrMsg: "invalid PodCliqueTemplateSpec name",
		},
		{
			description:      "Scaling group with long names",
			pgsName:          "verylongpodgangsetname",
			cliqueNames:      []string{"verylongpodcliquename"},
			scalingGroups:    []grovecorev1alpha1.PodCliqueScalingGroupConfig{createScalingGroupConfig("verylongscalinggroup", []string{"verylongpodcliquename"})},
			expectError:      true,
			expectedErrMsg:   "combined resource name length",
			expectedErrCount: 3,
		},
		{
			description:    "Scaling group referencing non-existent PodClique",
			pgsName:        "inference",
			cliqueNames:    []string{"prefill"},
			scalingGroups:  []grovecorev1alpha1.PodCliqueScalingGroupConfig{createScalingGroupConfig("workers", []string{"nonexistent"})},
			expectError:    true,
			expectedErrMsg: "unidentified PodClique names found",
		},
		{
			description:   "Maximum valid character usage",
			pgsName:       "pgs",
			cliqueNames:   []string{"cliquename20charssss"},
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{createScalingGroupConfig("sg", []string{"cliquename20charssss"})},
			expectError:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			pgsBuilder := testutils.NewPodGangSetBuilder(tc.pgsName, "default").
				WithReplicas(1).
				WithTerminationDelay(30 * time.Second).
				WithCliqueStartupType(ptr.To(grovecorev1alpha1.CliqueStartupTypeAnyOrder))

			// Add PodClique templates
			for _, cliqueName := range tc.cliqueNames {
				clique := testutils.NewPodCliqueTemplateSpecBuilder(cliqueName).
					WithReplicas(1).
					WithRoleName(fmt.Sprintf("dummy-%s-role", cliqueName)).
					WithMinAvailable(1).
					Build()
				pgsBuilder = pgsBuilder.WithPodCliqueTemplateSpec(clique)
			}

			// Add scaling groups
			for _, config := range tc.scalingGroups {
				pgsBuilder = pgsBuilder.WithPodCliqueScalingGroupConfig(config)
			}

			pgs := pgsBuilder.Build()

			validator := newPGSValidator(pgs, admissionv1.Create)
			warnings, err := validator.validate()

			if tc.expectError {
				assert.Error(t, err, "Expected validation error for test case: %s", tc.description)
				assert.Contains(t, err.Error(), tc.expectedErrMsg, "Error message should contain expected text")
				if tc.expectedErrCount > 0 {
					var aggErr utilerrors.Aggregate
					if errors.As(err, &aggErr) {
						assert.Len(t, aggErr.Errors(), tc.expectedErrCount, "Expected specific number of validation errors")
					}
				}
			} else {
				assert.NoError(t, err, "Expected no validation error for test case: %s", tc.description)
			}

			assert.Empty(t, warnings, "No warnings expected for these test cases")
		})
	}
}

func TestPodCliqueScalingGroupConfigValidation(t *testing.T) {
	testCases := []struct {
		description     string
		pgsName         string
		scalingGroups   []grovecorev1alpha1.PodCliqueScalingGroupConfig
		cliqueTemplates []string
		expectError     bool
		expectedErrMsg  string
	}{
		{
			description: "Valid scaling group with Replicas and MinAvailable",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:         "workers",
					CliqueNames:  []string{"prefill"},
					Replicas:     ptr.To(int32(4)),
					MinAvailable: ptr.To(int32(2)),
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     false,
		},
		{
			description: "Invalid Replicas (negative value)",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:         "workers",
					CliqueNames:  []string{"prefill"},
					Replicas:     ptr.To(int32(-1)),
					MinAvailable: ptr.To(int32(1)),
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     true,
			expectedErrMsg:  "must be greater than 0",
		},
		{
			description: "Invalid MinAvailable (zero value)",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:         "workers",
					CliqueNames:  []string{"prefill"},
					Replicas:     ptr.To(int32(4)),
					MinAvailable: ptr.To(int32(0)),
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     true,
			expectedErrMsg:  "must be greater than 0",
		},
		{
			description: "Invalid MinAvailable > Replicas",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:         "workers",
					CliqueNames:  []string{"prefill"},
					Replicas:     ptr.To(int32(2)),
					MinAvailable: ptr.To(int32(4)),
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     true,
			expectedErrMsg:  "minAvailable must not be greater than replicas",
		},
		{
			description: "Invalid ScaleConfig.MinReplicas < MinAvailable",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:         "workers",
					CliqueNames:  []string{"prefill"},
					Replicas:     ptr.To(int32(4)),
					MinAvailable: ptr.To(int32(3)),
					ScaleConfig: &grovecorev1alpha1.AutoScalingConfig{
						MinReplicas: ptr.To(int32(2)),
						MaxReplicas: 10,
					},
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     true,
			expectedErrMsg:  "scaleConfig.minReplicas must be greater than or equal to minAvailable",
		},
		{
			description: "Valid with partial configuration",
			pgsName:     "inference",
			scalingGroups: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					Name:        "workers",
					CliqueNames: []string{"prefill"},
					Replicas:    ptr.To(int32(4)),
				},
			},
			cliqueTemplates: []string{"prefill"},
			expectError:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			pgs := createDummyPodGangSet(tc.pgsName)

			// Add PodClique templates
			for _, cliqueName := range tc.cliqueTemplates {
				pgs.Spec.Template.Cliques = append(pgs.Spec.Template.Cliques, createDummyPodCliqueTemplate(cliqueName))
			}

			// Add scaling groups
			pgs.Spec.Template.PodCliqueScalingGroupConfigs = tc.scalingGroups

			validator := newPGSValidator(pgs, admissionv1.Create)
			warnings, err := validator.validate()

			if tc.expectError {
				assert.Error(t, err, "Expected validation error for test case: %s", tc.description)
				assert.Contains(t, err.Error(), tc.expectedErrMsg, "Error message should contain expected text")
			} else {
				assert.NoError(t, err, "Expected no validation error for test case: %s", tc.description)
			}

			assert.Empty(t, warnings, "No warnings expected for these test cases")
		})
	}
}

func TestPodCliqueUpdateValidation(t *testing.T) {
	testCases := []struct {
		name           string
		startupType    *grovecorev1alpha1.CliqueStartupType
		oldCliques     []*grovecorev1alpha1.PodCliqueTemplateSpec
		newCliques     []*grovecorev1alpha1.PodCliqueTemplateSpec
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "Valid: same cliques in different order with AnyOrder",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeAnyOrder),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("decode"),
				createDummyPodCliqueTemplate("prefill"),
			},
			expectError: false,
		},
		{
			name:        "Invalid: adding new clique",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeAnyOrder),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			expectError:    true,
			expectedErrMsg: "not allowed to change clique composition",
		},
		{
			name:        "Invalid: removing clique",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeAnyOrder),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
			},
			expectError:    true,
			expectedErrMsg: "not allowed to change clique composition",
		},
		{
			name:        "Invalid: InOrder doesn't allow order change",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeInOrder),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("decode"),
				createDummyPodCliqueTemplate("prefill"),
			},
			expectError:    true,
			expectedErrMsg: "clique order cannot be changed when StartupType is InOrder or Explicit",
		},
		{
			name:        "Invalid: Explicit doesn't allow order change",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeExplicit),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("decode"),
				createDummyPodCliqueTemplate("prefill"),
			},
			expectError:    true,
			expectedErrMsg: "clique order cannot be changed when StartupType is InOrder or Explicit",
		},
		{
			name:        "Valid: InOrder allows same order",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeInOrder),
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("prefill"),
				createDummyPodCliqueTemplate("decode"),
			},
			expectError: false,
		},
		{
			name:        "Edge case: empty arrays",
			startupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeInOrder),
			oldCliques:  []*grovecorev1alpha1.PodCliqueTemplateSpec{},
			newCliques:  []*grovecorev1alpha1.PodCliqueTemplateSpec{},
			expectError: false,
		},
		{
			name:        "Edge case: nil StartupType allows order change",
			startupType: nil,
			oldCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("a"),
				createDummyPodCliqueTemplate("b"),
			},
			newCliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
				createDummyPodCliqueTemplate("b"),
				createDummyPodCliqueTemplate("a"),
			},
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fldPath := field.NewPath("spec").Child("template").Child("cliques")

			validationErrors := validatePodCliqueUpdate(tc.newCliques, tc.oldCliques, tc.startupType, fldPath)

			if tc.expectError {
				assert.NotEmpty(t, validationErrors, "Expected validation errors for test case: %s", tc.name)
				var errorMessages []string
				for _, err := range validationErrors {
					errorMessages = append(errorMessages, err.Error())
				}
				errorString := fmt.Sprintf("%v", errorMessages)
				assert.Contains(t, errorString, tc.expectedErrMsg, "Error message should contain expected text")
			} else {
				assert.Empty(t, validationErrors, "Expected no validation errors for test case: %s", tc.name)
			}
		})
	}
}

func TestImmutableFieldsValidation(t *testing.T) {
	testCases := []struct {
		name           string
		setupOldPGS    func() *grovecorev1alpha1.PodGangSet
		setupNewPGS    func() *grovecorev1alpha1.PodGangSet
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "Valid: PriorityClassName can be updated",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.PriorityClassName = "old-priority"
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.PriorityClassName = "new-priority"
				return pgs
			},
			expectError: false,
		},
		{
			name: "Invalid: RoleName is immutable",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.RoleName = "old-role"
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.RoleName = "new-role"
				return pgs
			},
			expectError:    true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "Invalid: MinAvailable is immutable",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = ptr.To(int32(1))
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = ptr.To(int32(2))
				return pgs
			},
			expectError:    true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "Invalid: StartsAfter is immutable",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.StartupType = ptr.To(grovecorev1alpha1.CliqueStartupTypeExplicit)
				pgs.Spec.Template.Cliques = append(pgs.Spec.Template.Cliques, createDummyPodCliqueTemplate("clique2"))
				pgs.Spec.Template.Cliques[0].Spec.StartsAfter = []string{}
				pgs.Spec.Template.Cliques[1].Spec.StartsAfter = []string{"test"}
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.StartupType = ptr.To(grovecorev1alpha1.CliqueStartupTypeExplicit)
				pgs.Spec.Template.Cliques = append(pgs.Spec.Template.Cliques, createDummyPodCliqueTemplate("clique2"))
				pgs.Spec.Template.Cliques[0].Spec.StartsAfter = []string{}
				pgs.Spec.Template.Cliques[1].Spec.StartsAfter = []string{"test", "another"}
				return pgs
			},
			expectError:    true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "Edge case: Multiple immutable field violations",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.RoleName = "old-role"
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = ptr.To(int32(1))
				pgs.Spec.Template.Cliques[0].Spec.StartsAfter = []string{"dep1"}
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.RoleName = "new-role"
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = ptr.To(int32(2))
				pgs.Spec.Template.Cliques[0].Spec.StartsAfter = []string{"dep1", "dep2"}
				return pgs
			},
			expectError:    true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "Edge case: nil MinAvailable changing to non-nil",
			setupOldPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = nil
				return pgs
			},
			setupNewPGS: func() *grovecorev1alpha1.PodGangSet {
				pgs := createDummyPodGangSet("test")
				pgs.Spec.Template.Cliques[0].Spec.MinAvailable = ptr.To(int32(1))
				return pgs
			},
			expectError:    true,
			expectedErrMsg: "field is immutable",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			oldPGS := tc.setupOldPGS()
			newPGS := tc.setupNewPGS()

			validator := newPGSValidator(newPGS, admissionv1.Update)
			err := validator.validateUpdate(oldPGS)

			if tc.expectError {
				assert.Error(t, err, "Expected validation error for test case: %s", tc.name)
				assert.Contains(t, err.Error(), tc.expectedErrMsg, "Error message should contain expected text")
			} else {
				assert.NoError(t, err, "Expected no validation error for test case: %s", tc.name)
			}
		})
	}
}

func TestPodCliqueScalingGroupConfigsUpdateValidation(t *testing.T) {
	tests := []struct {
		name           string
		oldConfigs     []grovecorev1alpha1.PodCliqueScalingGroupConfig
		newConfigs     []grovecorev1alpha1.PodCliqueScalingGroupConfig
		expectedErrors bool
		expectedErrMsg string
	}{
		{
			name: "same configs - should pass",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			expectedErrors: false,
		},
		{
			name: "different clique names - should fail",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique3"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			expectedErrors: true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "different min available - should fail",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1", "clique2"},
					MinAvailable: ptr.To(int32(2)),
				},
			},
			expectedErrors: true,
			expectedErrMsg: "field is immutable",
		},
		{
			name: "adding new config - should fail",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: ptr.To(int32(1)),
				},
				{
					CliqueNames:  []string{"clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			expectedErrors: true,
			expectedErrMsg: "not allowed to add or remove",
		},
		{
			name: "removing config - should fail",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: ptr.To(int32(1)),
				},
				{
					CliqueNames:  []string{"clique2"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			expectedErrors: true,
			expectedErrMsg: "not allowed to add or remove",
		},
		{
			name:           "nil to empty slice - should pass",
			oldConfigs:     nil,
			newConfigs:     []grovecorev1alpha1.PodCliqueScalingGroupConfig{},
			expectedErrors: false,
		},
		{
			name:           "empty slice to nil - should pass",
			oldConfigs:     []grovecorev1alpha1.PodCliqueScalingGroupConfig{},
			newConfigs:     nil,
			expectedErrors: false,
		},
		{
			name: "nil min available in both - should pass",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: nil,
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: nil,
				},
			},
			expectedErrors: false,
		},
		{
			name: "nil to non-nil min available - should fail",
			oldConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: nil,
				},
			},
			newConfigs: []grovecorev1alpha1.PodCliqueScalingGroupConfig{
				{
					CliqueNames:  []string{"clique1"},
					MinAvailable: ptr.To(int32(1)),
				},
			},
			expectedErrors: true,
			expectedErrMsg: "field is immutable",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fldPath := field.NewPath("spec", "template", "podCliqueScalingGroupConfigs")
			validationErrors := validatePodCliqueScalingGroupConfigsUpdate(tc.newConfigs, tc.oldConfigs, fldPath)

			if tc.expectedErrors {
				assert.NotEmpty(t, validationErrors, "Expected validation errors for test case: %s", tc.name)
				var errorMessages []string
				for _, err := range validationErrors {
					errorMessages = append(errorMessages, err.Error())
				}
				errorString := fmt.Sprintf("%v", errorMessages)
				assert.Contains(t, errorString, tc.expectedErrMsg, "Error message should contain expected text")
			} else {
				assert.Empty(t, validationErrors, "Expected no validation errors for test case: %s", tc.name)
			}
		})
	}
}
