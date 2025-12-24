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

package topology

import (
	"context"
	"testing"

	kaitopologyv1alpha1 "github.com/NVIDIA/KAI-scheduler/pkg/apis/kai/v1alpha1"
	configv1alpha1 "github.com/ai-dynamo/grove/operator/api/config/v1alpha1"
	corev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConvertTopologyLevels(t *testing.T) {
	configLevels := []configv1alpha1.TopologyLevel{
		{Domain: "region", Key: "topology.kubernetes.io/region"},
		{Domain: "zone", Key: "topology.kubernetes.io/zone"},
	}

	coreLevels := convertTopologyLevels(configLevels)

	require.Len(t, coreLevels, 2)
	assert.Equal(t, corev1alpha1.TopologyDomain("region"), coreLevels[0].Domain)
	assert.Equal(t, "topology.kubernetes.io/region", coreLevels[0].Key)
	assert.Equal(t, corev1alpha1.TopologyDomain("zone"), coreLevels[1].Domain)
	assert.Equal(t, "topology.kubernetes.io/zone", coreLevels[1].Key)
}

func TestConvertClusterTopologyToKai(t *testing.T) {
	clusterTopology := &corev1alpha1.ClusterTopology{
		ObjectMeta: metav1.ObjectMeta{
			Name: "grove-topology",
		},
		Spec: corev1alpha1.ClusterTopologySpec{
			Levels: []corev1alpha1.TopologyLevel{
				{Domain: "zone", Key: "topology.kubernetes.io/zone"},
				{Domain: "region", Key: "topology.kubernetes.io/region"},
			},
		},
	}

	kaiTopology := convertClusterTopologyToKai(clusterTopology)

	assert.Equal(t, "grove-topology", kaiTopology.Name)
	require.Len(t, kaiTopology.Spec.Levels, 2)
	assert.Equal(t, "topology.kubernetes.io/region", kaiTopology.Spec.Levels[0].NodeLabel)
	assert.Equal(t, "topology.kubernetes.io/zone", kaiTopology.Spec.Levels[1].NodeLabel)
}

func TestEnsureTopology_Create(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = kaitopologyv1alpha1.AddToScheme(scheme)
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	levels := []configv1alpha1.TopologyLevel{
		{Domain: "zone", Key: "topology.kubernetes.io/zone"},
	}

	err := EnsureTopology(context.Background(), client, "test-topology", levels)
	assert.NoError(t, err)

	ct := &corev1alpha1.ClusterTopology{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-topology"}, ct)
	assert.NoError(t, err)
	assert.Len(t, ct.Spec.Levels, 1)

	kaiTopology := &kaitopologyv1alpha1.Topology{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-topology"}, kaiTopology)
	assert.NoError(t, err)
	assert.Len(t, kaiTopology.Spec.Levels, 1)
	assert.Equal(t, "topology.kubernetes.io/zone", kaiTopology.Spec.Levels[0].NodeLabel)
}

func TestEnsureTopology_Update(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1alpha1.AddToScheme(scheme)
	_ = kaitopologyv1alpha1.AddToScheme(scheme)

	existing := &corev1alpha1.ClusterTopology{
		ObjectMeta: metav1.ObjectMeta{Name: "test-topology"},
		Spec: corev1alpha1.ClusterTopologySpec{
			Levels: []corev1alpha1.TopologyLevel{
				{Domain: "zone", Key: "old-key"},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()

	levels := []configv1alpha1.TopologyLevel{
		{Domain: "zone", Key: "new-key"},
	}

	err := EnsureTopology(context.Background(), client, "test-topology", levels)
	assert.NoError(t, err)

	ct := &corev1alpha1.ClusterTopology{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-topology"}, ct)
	assert.NoError(t, err)
	assert.Equal(t, "new-key", ct.Spec.Levels[0].Key)

	kaiTopology := &kaitopologyv1alpha1.Topology{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-topology"}, kaiTopology)
	assert.NoError(t, err)
	assert.Len(t, kaiTopology.Spec.Levels, 1)
	assert.Equal(t, "new-key", kaiTopology.Spec.Levels[0].NodeLabel)
}
