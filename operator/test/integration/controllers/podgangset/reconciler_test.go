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

package podgangset

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PodGangSet Controller", func() {
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

	Context("When creating a simple PodGangSet", func() {
		var podGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-simple-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 2)).
				Build()
		})

		It("should create the PodGangSet successfully", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodGangSet exists")
			helpers.ExpectResourceEventuallyExists(ctx, k8sClient, podGangSet)

			By("checking initial status")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return ""
				}
				return string(podGangSet.Status.Phase)
			}).Should(Equal(string(grovecorev1alpha1.PodGangSetPending)))
		})

		It("should create child PodCliques", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for child PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))

			By("verifying PodClique properties")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(podCliqueList.Items).To(HaveLen(1))

			podClique := &podCliqueList.Items[0]
			Expect(podClique.Spec.Replicas).To(Equal(ptr.To(int32(2))))
			Expect(podClique.Name).To(ContainSubstring("worker"))
		})

		It("should update status when PodCliques become ready", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodClique to be created")
			var createdPodClique *grovecorev1alpha1.PodClique
			Eventually(func() bool {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil || len(podCliqueList.Items) == 0 {
					return false
				}
				createdPodClique = &podCliqueList.Items[0]
				return true
			}).Should(BeTrue())

			By("simulating PodClique becoming ready")
			createdPodClique.Status.Phase = grovecorev1alpha1.PodCliqueRunning
			createdPodClique.Status.ReadyReplicas = 2
			createdPodClique.Status.Replicas = 2
			err = k8sClient.Status().Update(ctx, createdPodClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PodGangSet status updates")
			helpers.ExpectPhaseEventually(ctx, k8sClient, podGangSet, string(grovecorev1alpha1.PodGangSetRunning))

			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return -1
				}
				return podGangSet.Status.ReadyReplicas
			}).Should(Equal(int32(2)))
		})
	})

	Context("When creating a PodGangSet with multiple replicas", func() {
		var podGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-multi-replica-pgs").
				WithReplicas(2).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()
		})

		It("should create multiple replica sets", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(2))

			By("verifying replica naming")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
			Expect(err).NotTo(HaveOccurred())

			names := make([]string, len(podCliqueList.Items))
			for i, pc := range podCliqueList.Items {
				names[i] = pc.Name
			}
			Expect(names).To(ContainElements(
				ContainSubstring("worker-0"),
				ContainSubstring("worker-1"),
			))
		})
	})

	Context("When creating a PodGangSet with multiple PodCliques", func() {
		var podGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-multi-clique-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("prefill", 1)).
				WithPodClique(helpers.SimpleGangSetPodClique("decode", 2)).
				Build()
		})

		It("should create all PodCliques", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for all PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(2))

			By("verifying PodClique configurations")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
			Expect(err).NotTo(HaveOccurred())

			cliquesByName := make(map[string]*grovecorev1alpha1.PodClique)
			for i, pc := range podCliqueList.Items {
				if pc.Name == podGangSet.Name+"-prefill-0" {
					cliquesByName["prefill"] = &podCliqueList.Items[i]
				} else if pc.Name == podGangSet.Name+"-decode-0" {
					cliquesByName["decode"] = &podCliqueList.Items[i]
				}
			}

			Expect(cliquesByName).To(HaveKey("prefill"))
			Expect(cliquesByName).To(HaveKey("decode"))
			Expect(cliquesByName["prefill"].Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(cliquesByName["decode"].Spec.Replicas).To(Equal(ptr.To(int32(2))))
		})
	})

	Context("When deleting a PodGangSet", func() {
		var podGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-delete-pgs").
				WithReplicas(1).
				WithPodClique(helpers.SimpleGangSetPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))
		})

		It("should delete child PodCliques", func() {
			By("deleting the PodGangSet")
			err := k8sClient.Delete(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodGangSet to be deleted")
			helpers.ExpectResourceEventuallyDeleted(ctx, k8sClient, podGangSet)

			By("verifying child PodCliques are deleted")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(0))
		})
	})

	Context("When handling errors", func() {
		It("should handle invalid PodClique configurations", func() {
			By("creating a PodGangSet with invalid container spec")
			podGangSet := helpers.NewPodGangSetBuilder().
				WithName("test-invalid-pgs").
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
									Image: "invalid-image",
								},
							},
						},
					},
				},
			}

			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("verifying error conditions are set")
			helpers.ExpectConditionEventually(ctx, k8sClient, podGangSet,
				"Available", metav1.ConditionFalse, "")
		})
	})
})
