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
	"context"
	"fmt"

	"github.com/ai-dynamo/grove/operator/api/common/constants"
	grovecorev1alpha1 "github.com/ai-dynamo/grove/operator/api/core/v1alpha1"
	k8sutils "github.com/ai-dynamo/grove/operator/internal/utils/kubernetes"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// PodOwnerPCLQField is the field index key for looking up pods by their owning PodClique.
// The index must be registered via RegisterPodOwnerPCLQIndex before the manager starts.
const PodOwnerPCLQField = ".metadata.controller-pclq"

// RegisterPodOwnerPCLQIndex registers a field index on Pods by their owning PodClique name.
// This enables efficient cache lookups in GetPCLQPods instead of scanning all pods.
// Must be called before the manager starts.
func RegisterPodOwnerPCLQIndex(ctx context.Context, mgr manager.Manager) error {
	return mgr.GetFieldIndexer().IndexField(ctx, &corev1.Pod{}, PodOwnerPCLQField, func(obj client.Object) []string {
		pod, ok := obj.(*corev1.Pod)
		if !ok {
			return nil
		}
		ownerRef := k8sutils.FindOwnerRefByKind(pod.OwnerReferences, constants.KindPodClique)
		if ownerRef == nil {
			return nil
		}
		return []string{fmt.Sprintf("%s/%s", pod.Namespace, ownerRef.Name)}
	})
}

// GetPCLQPods lists all Pods owned by a PodClique using a field index for efficient lookup.
func GetPCLQPods(ctx context.Context, cl client.Client, _ string, pclq *grovecorev1alpha1.PodClique) ([]*corev1.Pod, error) {
	podList := &corev1.PodList{}
	indexKey := fmt.Sprintf("%s/%s", pclq.Namespace, pclq.Name)
	if err := cl.List(ctx,
		podList,
		client.InNamespace(pclq.Namespace),
		client.MatchingFields{PodOwnerPCLQField: indexKey},
	); err != nil {
		return nil, err
	}
	ownedPods := make([]*corev1.Pod, 0, len(podList.Items))
	for _, pod := range podList.Items {
		if metav1.IsControlledBy(&pod, pclq) {
			ownedPods = append(ownedPods, &pod)
		}
	}
	return ownedPods, nil
}

// AddEnvVarsToContainers adds the given environment variables to the Pod containers.
func AddEnvVarsToContainers(containers []corev1.Container, envVars []corev1.EnvVar) {
	for i := range containers {
		containers[i].Env = append(containers[i].Env, envVars...)
	}
}

// PodsToObjectNames converts a slice of Pods to a slice of string representations in "namespace/name" format.
func PodsToObjectNames(pods []*corev1.Pod) []string {
	return lo.Map(pods, func(pod *corev1.Pod, _ int) string {
		return cache.NamespacedNameAsObjectName(client.ObjectKeyFromObject(pod)).String()
	})
}
