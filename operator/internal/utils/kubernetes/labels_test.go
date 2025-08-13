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

package kubernetes

import (
	"errors"
	"testing"

	grovev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Test helper functions
func newTestObjectMetaWithReplicaIndex(index string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      "test-resource",
		Namespace: "test-ns",
		Labels: map[string]string{
			grovev1alpha1.LabelPodGangSetReplicaIndex: index,
		},
	}
}

func newTestObjectMetaWithLabels(labels map[string]string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      "test-resource",
		Namespace: "test-ns",
		Labels:    labels,
	}
}

func newTestObjectMetaNilLabels() metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      "test-resource",
		Namespace: "test-ns",
		Labels:    nil,
	}
}

func newLabelsWithReplicaIndex(index string) map[string]string {
	return map[string]string{
		grovev1alpha1.LabelPodGangSetReplicaIndex: index,
	}
}

func newLabelsWithReplicaIndexAndExtras(index string, extraLabels map[string]string) map[string]string {
	labels := newLabelsWithReplicaIndex(index)
	for k, v := range extraLabels {
		labels[k] = v
	}
	return labels
}

func TestGetPodGangSetReplicaIndex(t *testing.T) {
	testCases := []struct {
		description   string
		objMeta       metav1.ObjectMeta
		expectedIndex int
		expectedError error
	}{
		{
			description:   "valid replica index 0",
			objMeta:       newTestObjectMetaWithReplicaIndex("0"),
			expectedIndex: 0,
			expectedError: nil,
		},
		{
			description:   "valid replica index 1",
			objMeta:       newTestObjectMetaWithReplicaIndex("1"),
			expectedIndex: 1,
			expectedError: nil,
		},
		{
			description:   "valid replica index 42",
			objMeta:       newTestObjectMetaWithReplicaIndex("42"),
			expectedIndex: 42,
			expectedError: nil,
		},
		{
			description: "valid replica index with multiple labels",
			objMeta: newTestObjectMetaWithLabels(newLabelsWithReplicaIndexAndExtras("5", map[string]string{
				"app":         "my-app",
				"version":     "v1.0",
				"environment": "test",
			})),
			expectedIndex: 5,
			expectedError: nil,
		},
		{
			description: "missing replica index label",
			objMeta: newTestObjectMetaWithLabels(map[string]string{
				"app":     "my-app",
				"version": "v1.0",
			}),
			expectedIndex: 0,
			expectedError: errNotFoundPodGangSetReplicaIndexLabel,
		},
		{
			description:   "nil labels map",
			objMeta:       newTestObjectMetaNilLabels(),
			expectedIndex: 0,
			expectedError: errNotFoundPodGangSetReplicaIndexLabel,
		},
		{
			description:   "empty labels map",
			objMeta:       newTestObjectMetaWithLabels(map[string]string{}),
			expectedIndex: 0,
			expectedError: errNotFoundPodGangSetReplicaIndexLabel,
		},
		{
			description:   "invalid replica index - non-numeric string",
			objMeta:       newTestObjectMetaWithReplicaIndex("abc"),
			expectedIndex: 0,
			expectedError: errReplicaIndexIntConversion,
		},
		{
			description:   "invalid replica index - empty string",
			objMeta:       newTestObjectMetaWithReplicaIndex(""),
			expectedIndex: 0,
			expectedError: errReplicaIndexIntConversion,
		},
		{
			description:   "invalid replica index - float string",
			objMeta:       newTestObjectMetaWithReplicaIndex("1.5"),
			expectedIndex: 0,
			expectedError: errReplicaIndexIntConversion,
		},
		{
			description:   "invalid replica index - negative number",
			objMeta:       newTestObjectMetaWithReplicaIndex("-1"),
			expectedIndex: -1,
			expectedError: nil,
		},
		{
			description:   "valid replica index - large number",
			objMeta:       newTestObjectMetaWithReplicaIndex("9999"),
			expectedIndex: 9999,
			expectedError: nil,
		},
		{
			description:   "invalid replica index - whitespace",
			objMeta:       newTestObjectMetaWithReplicaIndex("  "),
			expectedIndex: 0,
			expectedError: errReplicaIndexIntConversion,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			index, err := GetPodGangSetReplicaIndex(tc.objMeta)

			if tc.expectedError != nil {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, tc.expectedError),
					"expected error %v to contain %v", err, tc.expectedError)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tc.expectedIndex, index)
		})
	}
}
