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

package validation

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PodGangSet Validation Webhook", func() {
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

	Context("When validating PodGangSet creation", func() {
		It("should accept valid PodGangSet", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("valid-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject PodGangSet with invalid name length", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("this-is-a-very-long-podgangset-name-that-exceeds-the-45-character-limit-for-resource-names").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject PodGangSet with zero replicas", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("zero-replicas-pgs").
				WithReplicas(0).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject PodGangSet with negative replicas", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("negative-replicas-pgs").
				WithReplicas(-1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject PodGangSet with empty PodCliques", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("empty-cliques-pgs").
				WithReplicas(1).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject PodGangSet with duplicate PodClique names", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("duplicate-names-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 2)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject PodGangSet with invalid container specification", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("invalid-container-pgs").
				WithReplicas(1).
				Build()

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				{
					Name:     "invalid",
					Replicas: ptr.To(int32(1)),
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "",
									Image: "",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})
	})

	Context("When validating role dependencies", func() {
		It("should accept valid role dependencies", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("valid-dependencies-pgs").
				WithReplicas(1).
				Build()

			primaryClique := helpers.SimpleGangSetPodClique("primary", 1)
			primaryClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:        "leader",
				StartupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
			}

			workerClique := helpers.SimpleGangSetPodClique("worker", 1)
			workerClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "follower",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"leader"},
			}

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				primaryClique,
				workerClique,
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject circular dependencies", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("circular-deps-pgs").
				WithReplicas(1).
				Build()

			clique1 := helpers.SimpleGangSetPodClique("clique1", 1)
			clique1.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "role1",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"role2"},
			}

			clique2 := helpers.SimpleGangSetPodClique("clique2", 1)
			clique2.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "role2",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"role1"},
			}

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				clique1,
				clique2,
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject dependencies on non-existent roles", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("missing-deps-pgs").
				WithReplicas(1).
				Build()

			workerClique := helpers.SimpleGangSetPodClique("worker", 1)
			workerClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "worker",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"non-existent-role"},
			}

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				workerClique,
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject duplicate role names", func() {
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("duplicate-roles-pgs").
				WithReplicas(1).
				Build()

			clique1 := helpers.SimpleGangSetPodClique("clique1", 1)
			clique1.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:        "duplicate-role",
				StartupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeParallel),
			}

			clique2 := helpers.SimpleGangSetPodClique("clique2", 1)
			clique2.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:        "duplicate-role",
				StartupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeParallel),
			}

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				clique1,
				clique2,
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})
	})

	Context("When validating PodGangSet updates", func() {
		var existingPodGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			existingPodGangSet = helpers.NewPodGangSetBuilder().
				WithName("update-test-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, existingPodGangSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should allow valid updates", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingPodGangSet), existingPodGangSet)
			Expect(err).NotTo(HaveOccurred())

			existingPodGangSet.Spec.Replicas = ptr.To(int32(2))
			err = k8sClient.Update(ctx, existingPodGangSet)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject invalid replica updates", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingPodGangSet), existingPodGangSet)
			Expect(err).NotTo(HaveOccurred())

			existingPodGangSet.Spec.Replicas = ptr.To(int32(-1))
			err = k8sClient.Update(ctx, existingPodGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})

		It("should reject updates that create invalid role dependencies", func() {
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(existingPodGangSet), existingPodGangSet)
			Expect(err).NotTo(HaveOccurred())

			invalidClique := helpers.SimpleGangSetPodClique("invalid", 1)
			invalidClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "invalid-role",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"non-existent"},
			}

			existingPodGangSet.Spec.PodCliques = append(existingPodGangSet.Spec.PodCliques, invalidClique)
			err = k8sClient.Update(ctx, existingPodGangSet)
			Expect(err).To(HaveOccurred())
			Expect(apierrors.IsInvalid(err)).To(BeTrue())
		})
	})
})
