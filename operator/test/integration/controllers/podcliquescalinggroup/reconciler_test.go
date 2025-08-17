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

package podcliquescalinggroup

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PodCliqueScalingGroup Controller", func() {
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

	Context("When creating a simple PodCliqueScalingGroup", func() {
		var scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-simple-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("worker", 2)).
				Build()
		})

		It("should create the PodCliqueScalingGroup successfully", func() {
			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodCliqueScalingGroup exists")
			helpers.ExpectResourceEventuallyExists(ctx, k8sClient, scalingGroup)

			By("checking initial status")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
				if err != nil {
					return ""
				}
				return string(scalingGroup.Status.Phase)
			}).Should(Equal(string(grovecorev1alpha1.PodCliqueScalingGroupPending)))
		})

		It("should create child PodCliques", func() {
			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for child PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))

			By("verifying PodClique properties")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(podCliqueList.Items).To(HaveLen(1))

			podClique := &podCliqueList.Items[0]
			Expect(podClique.Spec.Replicas).To(Equal(ptr.To(int32(2))))
			Expect(podClique.Name).To(ContainSubstring("worker"))
		})

		It("should update status when PodCliques become ready", func() {
			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodClique to be created")
			var createdPodClique *grovecorev1alpha1.PodClique
			Eventually(func() bool {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
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

			By("verifying PodCliqueScalingGroup status updates")
			helpers.ExpectPhaseEventually(ctx, k8sClient, scalingGroup, string(grovecorev1alpha1.PodCliqueScalingGroupRunning))

			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
				if err != nil {
					return -1
				}
				return scalingGroup.Status.ReadyReplicas
			}).Should(Equal(int32(2)))
		})
	})

	Context("When creating a PodCliqueScalingGroup with multiple PodCliques", func() {
		var scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-multi-clique-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("prefill", 1)).
				WithPodClique(helpers.SimpleScalingGroupPodClique("decode", 2)).
				Build()
		})

		It("should create all PodCliques", func() {
			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for all PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(2))

			By("verifying PodClique configurations")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
			Expect(err).NotTo(HaveOccurred())

			cliquesByName := make(map[string]*grovecorev1alpha1.PodClique)
			for i, pc := range podCliqueList.Items {
				if pc.Name == scalingGroup.Name+"-prefill" {
					cliquesByName["prefill"] = &podCliqueList.Items[i]
				} else if pc.Name == scalingGroup.Name+"-decode" {
					cliquesByName["decode"] = &podCliqueList.Items[i]
				}
			}

			Expect(cliquesByName).To(HaveKey("prefill"))
			Expect(cliquesByName).To(HaveKey("decode"))
			Expect(cliquesByName["prefill"].Spec.Replicas).To(Equal(ptr.To(int32(1))))
			Expect(cliquesByName["decode"].Spec.Replicas).To(Equal(ptr.To(int32(2))))
		})

		It("should coordinate PodClique startup", func() {
			By("creating the PodCliqueScalingGroup")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodCliques to be created")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(2))

			By("verifying all PodCliques have proper startup coordination")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
			Expect(err).NotTo(HaveOccurred())

			for _, pc := range podCliqueList.Items {
				Expect(pc.Labels).To(HaveKey(common.LabelPodCliqueScalingGroupName))
				Expect(pc.Labels[common.LabelPodCliqueScalingGroupName]).To(Equal(scalingGroup.Name))
			}
		})
	})

	Context("When scaling a PodCliqueScalingGroup", func() {
		var scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-scaling-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))
		})

		It("should update PodClique replicas when scaling group spec changes", func() {
			By("updating PodClique replica count in scaling group")
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			scalingGroup.Spec.PodCliques[0].Replicas = ptr.To(int32(3))
			err = k8sClient.Update(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodClique to be updated")
			Eventually(func() int32 {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil || len(podCliqueList.Items) == 0 {
					return -1
				}

				pc := &podCliqueList.Items[0]
				if pc.Spec.Replicas == nil {
					return -1
				}
				return *pc.Spec.Replicas
			}).Should(Equal(int32(3)))

			By("verifying status reflects new replica count")
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
				if err != nil {
					return -1
				}
				return scalingGroup.Status.Replicas
			}).Should(Equal(int32(3)))
		})
	})

	Context("When deleting a PodCliqueScalingGroup", func() {
		var scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-delete-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("worker", 1)).
				Build()

			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))
		})

		It("should delete child PodCliques", func() {
			By("deleting the PodCliqueScalingGroup")
			err := k8sClient.Delete(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodCliqueScalingGroup to be deleted")
			helpers.ExpectResourceEventuallyDeleted(ctx, k8sClient, scalingGroup)

			By("verifying child PodCliques are deleted")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(0))
		})
	})

	Context("When handling coordination scenarios", func() {
		var scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-coordination-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("primary", 1)).
				WithPodClique(helpers.SimpleScalingGroupPodClique("secondary", 2)).
				Build()

			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(2))
		})

		It("should coordinate status across multiple PodCliques", func() {
			By("getting created PodCliques")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err := k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(podCliqueList.Items).To(HaveLen(2))

			By("simulating one PodClique becoming ready")
			primaryClique := &podCliqueList.Items[0]
			primaryClique.Status.Phase = grovecorev1alpha1.PodCliqueRunning
			primaryClique.Status.ReadyReplicas = 1
			primaryClique.Status.Replicas = 1
			err = k8sClient.Status().Update(ctx, primaryClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying ScalingGroup is still pending")
			Consistently(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
				if err != nil {
					return ""
				}
				return string(scalingGroup.Status.Phase)
			}).Should(Equal(string(grovecorev1alpha1.PodCliqueScalingGroupPending)))

			By("simulating second PodClique becoming ready")
			secondaryClique := &podCliqueList.Items[1]
			secondaryClique.Status.Phase = grovecorev1alpha1.PodCliqueRunning
			secondaryClique.Status.ReadyReplicas = 2
			secondaryClique.Status.Replicas = 2
			err = k8sClient.Status().Update(ctx, secondaryClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying ScalingGroup becomes running")
			helpers.ExpectPhaseEventually(ctx, k8sClient, scalingGroup, string(grovecorev1alpha1.PodCliqueScalingGroupRunning))

			By("verifying total ready replicas")
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(scalingGroup), scalingGroup)
				if err != nil {
					return -1
				}
				return scalingGroup.Status.ReadyReplicas
			}).Should(Equal(int32(3)))
		})
	})
})
