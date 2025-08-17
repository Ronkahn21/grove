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

package integration

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Basic Integration Tests", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		namespace string
	)

	BeforeEach(func() {
		ctx = context.Background()
		k8sClient = helpers.GetTestClient()
		namespace = helpers.GetTestNamespace()
	})

	Context("When testing basic functionality", func() {
		It("should create a simple PodClique", func() {
			podClique := &grovecorev1alpha1.PodClique{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-basic-podclique",
					Namespace: namespace,
				},
				Spec: grovecorev1alpha1.PodCliqueSpec{
					RoleName: "worker",
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
			}

			By("creating the PodClique")
			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodClique exists")
			key := types.NamespacedName{Name: podClique.Name, Namespace: podClique.Namespace}
			retrievedPodClique := &grovecorev1alpha1.PodClique{}

			Eventually(func() error {
				return k8sClient.Get(ctx, key, retrievedPodClique)
			}).Should(Succeed())

			By("verifying the PodClique properties")
			Expect(retrievedPodClique.Spec.RoleName).To(Equal("worker"))
			Expect(retrievedPodClique.Spec.Replicas).To(Equal(int32(1)))
			Expect(retrievedPodClique.Spec.PodSpec.Containers).To(HaveLen(1))
		})

		It("should create a simple PodCliqueScalingGroup", func() {
			scalingGroup := &grovecorev1alpha1.PodCliqueScalingGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-basic-scaling-group",
					Namespace: namespace,
				},
				Spec: grovecorev1alpha1.PodCliqueScalingGroupSpec{
					Replicas:    1,
					CliqueNames: []string{"worker"},
				},
			}

			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodCliqueScalingGroup exists")
			key := types.NamespacedName{Name: scalingGroup.Name, Namespace: scalingGroup.Namespace}
			retrievedScalingGroup := &grovecorev1alpha1.PodCliqueScalingGroup{}

			Eventually(func() error {
				return k8sClient.Get(ctx, key, retrievedScalingGroup)
			}).Should(Succeed())

			By("verifying the PodCliqueScalingGroup properties")
			Expect(retrievedScalingGroup.Spec.Replicas).To(Equal(int32(1)))
			Expect(retrievedScalingGroup.Spec.CliqueNames).To(Equal([]string{"worker"}))
		})

		It("should create a simple PodGangSet", func() {
			podGangSet := &grovecorev1alpha1.PodGangSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-basic-podgangset",
					Namespace: namespace,
				},
				Spec: grovecorev1alpha1.PodGangSetSpec{
					Replicas: 1,
					Template: grovecorev1alpha1.PodGangSetTemplateSpec{
						Cliques: []*grovecorev1alpha1.PodCliqueTemplateSpec{
							{
								Name: "worker",
								Spec: grovecorev1alpha1.PodCliqueSpec{
									RoleName: "worker",
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
						},
					},
				},
			}

			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodGangSet exists")
			key := types.NamespacedName{Name: podGangSet.Name, Namespace: podGangSet.Namespace}
			retrievedPodGangSet := &grovecorev1alpha1.PodGangSet{}

			Eventually(func() error {
				return k8sClient.Get(ctx, key, retrievedPodGangSet)
			}).Should(Succeed())

			By("verifying the PodGangSet properties")
			Expect(retrievedPodGangSet.Spec.Replicas).To(Equal(int32(1)))
			Expect(retrievedPodGangSet.Spec.Template.Cliques).To(HaveLen(1))
			Expect(retrievedPodGangSet.Spec.Template.Cliques[0].Name).To(Equal("worker"))
		})
	})
})
