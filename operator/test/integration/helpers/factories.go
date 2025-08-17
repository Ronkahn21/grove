// /*
// Copyright 2024 The Grove Authors.
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

package helpers

import (
	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodGangSetBuilder struct {
	pgs *grovecorev1alpha1.PodGangSet
}

func NewPodGangSetBuilder() *PodGangSetBuilder {
	return &PodGangSetBuilder{
		pgs: &grovecorev1alpha1.PodGangSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-podgangset-" + uuid.New().String()[:8],
				Namespace: GetTestNamespace(),
			},
			Spec: grovecorev1alpha1.PodGangSetSpec{
				Replicas: 1,
				Template: grovecorev1alpha1.PodGangSetTemplateSpec{
					Cliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{},
				},
			},
		},
	}
}

func (b *PodGangSetBuilder) WithName(name string) *PodGangSetBuilder {
	b.pgs.Name = name
	return b
}

func (b *PodGangSetBuilder) WithNamespace(namespace string) *PodGangSetBuilder {
	b.pgs.Namespace = namespace
	return b
}

func (b *PodGangSetBuilder) WithReplicas(replicas int32) *PodGangSetBuilder {
	b.pgs.Spec.Replicas = replicas
	return b
}

func (b *PodGangSetBuilder) WithPodClique(clique *grovecorev1alpha1.PodCliqueTemplateSpec) *PodGangSetBuilder {
	b.pgs.Spec.Template.Cliques = append(b.pgs.Spec.Template.Cliques, clique)
	return b
}

func (b *PodGangSetBuilder) Build() *grovecorev1alpha1.PodGangSet {
	return b.pgs.DeepCopy()
}

type PodCliqueBuilder struct {
	pc *grovecorev1alpha1.PodClique
}

func NewPodCliqueBuilder() *PodCliqueBuilder {
	return &PodCliqueBuilder{
		pc: &grovecorev1alpha1.PodClique{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-podclique-" + uuid.New().String()[:8],
				Namespace: GetTestNamespace(),
			},
			Spec: grovecorev1alpha1.PodCliqueSpec{
				RoleName: "test-role",
				Replicas: 1,
				PodSpec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "nginx:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

func (b *PodCliqueBuilder) WithName(name string) *PodCliqueBuilder {
	b.pc.Name = name
	return b
}

func (b *PodCliqueBuilder) WithNamespace(namespace string) *PodCliqueBuilder {
	b.pc.Namespace = namespace
	return b
}

func (b *PodCliqueBuilder) WithReplicas(replicas int32) *PodCliqueBuilder {
	b.pc.Spec.Replicas = replicas
	return b
}

func (b *PodCliqueBuilder) WithContainer(container corev1.Container) *PodCliqueBuilder {
	b.pc.Spec.PodSpec.Containers = []corev1.Container{container}
	return b
}

func (b *PodCliqueBuilder) WithRole(role string) *PodCliqueBuilder {
	b.pc.Spec.RoleName = role
	return b
}

func (b *PodCliqueBuilder) WithStartsAfter(startsAfter []string) *PodCliqueBuilder {
	b.pc.Spec.StartsAfter = startsAfter
	return b
}

func (b *PodCliqueBuilder) Build() *grovecorev1alpha1.PodClique {
	return b.pc.DeepCopy()
}

type PodCliqueScalingGroupBuilder struct {
	sg *grovecorev1alpha1.PodCliqueScalingGroup
}

func NewPodCliqueScalingGroupBuilder() *PodCliqueScalingGroupBuilder {
	return &PodCliqueScalingGroupBuilder{
		sg: &grovecorev1alpha1.PodCliqueScalingGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-scalinggroup-" + uuid.New().String()[:8],
				Namespace: GetTestNamespace(),
			},
			Spec: grovecorev1alpha1.PodCliqueScalingGroupSpec{
				Replicas:    1,
				CliqueNames: []string{},
			},
		},
	}
}

func (b *PodCliqueScalingGroupBuilder) WithName(name string) *PodCliqueScalingGroupBuilder {
	b.sg.Name = name
	return b
}

func (b *PodCliqueScalingGroupBuilder) WithNamespace(namespace string) *PodCliqueScalingGroupBuilder {
	b.sg.Namespace = namespace
	return b
}

func (b *PodCliqueScalingGroupBuilder) WithCliqueName(cliqueName string) *PodCliqueScalingGroupBuilder {
	b.sg.Spec.CliqueNames = append(b.sg.Spec.CliqueNames, cliqueName)
	return b
}

func (b *PodCliqueScalingGroupBuilder) Build() *grovecorev1alpha1.PodCliqueScalingGroup {
	return b.sg.DeepCopy()
}

func DefaultContainer() corev1.Container {
	return corev1.Container{
		Name:  "test-container",
		Image: "nginx:latest",
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("256Mi"),
			},
		},
	}
}

func SimplePodCliqueTemplateSpec(name string, replicas int32) *grovecorev1alpha1.PodCliqueTemplateSpec {
	return &grovecorev1alpha1.PodCliqueTemplateSpec{
		Name: name,
		Spec: grovecorev1alpha1.PodCliqueSpec{
			RoleName: name,
			Replicas: replicas,
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{DefaultContainer()},
			},
		},
	}
}
