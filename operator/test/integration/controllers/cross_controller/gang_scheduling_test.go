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

package cross_controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Gang Scheduling Integration", func() {
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

	Context("When creating a PodGangSet with ScalingGroups", func() {
		var (
			podGangSet   *grovecorev1alpha1.PodGangSet
			scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup
		)

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-sg-for-gang").
				WithPodClique(helpers.SimpleScalingGroupPodClique("prefill", 1)).
				WithPodClique(helpers.SimpleScalingGroupPodClique("decode", 2)).
				Build()

			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-gang-with-sg").
				WithReplicas(1).
				Build()

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				{
					Name: "scaling-group-ref",
					ScalingGroupRef: &grovecorev1alpha1.PodGangSetScalingGroupRef{
						Name: scalingGroup.Name,
					},
				},
			}
		})

		It("should coordinate PodGangSet and ScalingGroup lifecycle", func() {
			By("creating the ScalingGroup first")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for ScalingGroup PodCliques to be created")
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

			By("creating the PodGangSet")
			err = k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PodGangSet references the ScalingGroup")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return false
				}

				return len(podGangSet.Spec.PodCliques) > 0 &&
					podGangSet.Spec.PodCliques[0].ScalingGroupRef != nil &&
					podGangSet.Spec.PodCliques[0].ScalingGroupRef.Name == scalingGroup.Name
			}).Should(BeTrue())

			By("verifying gang scheduling coordination")
			Eventually(func() bool {
				sgPodCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, sgPodCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil || len(sgPodCliqueList.Items) != 2 {
					return false
				}

				gangPodCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err = k8sClient.List(ctx, gangPodCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil {
					return false
				}

				return len(gangPodCliqueList.Items) >= 2
			}).Should(BeTrue())
		})

		It("should propagate status from ScalingGroup to PodGangSet", func() {
			By("creating both resources")
			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodCliques to be created")
			var sgPodCliques []grovecorev1alpha1.PodClique
			Eventually(func() bool {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: scalingGroup.Name})
				if err != nil || len(podCliqueList.Items) != 2 {
					return false
				}
				sgPodCliques = podCliqueList.Items
				return true
			}).Should(BeTrue())

			By("simulating ScalingGroup PodCliques becoming ready")
			for i := range sgPodCliques {
				pc := &sgPodCliques[i]
				pc.Status.Phase = grovecorev1alpha1.PodCliqueRunning
				if pc.Name == scalingGroup.Name+"-prefill" {
					pc.Status.ReadyReplicas = 1
					pc.Status.Replicas = 1
				} else {
					pc.Status.ReadyReplicas = 2
					pc.Status.Replicas = 2
				}
				err = k8sClient.Status().Update(ctx, pc)
				Expect(err).NotTo(HaveOccurred())
			}

			By("waiting for ScalingGroup to become ready")
			helpers.ExpectPhaseEventually(ctx, k8sClient, scalingGroup, string(grovecorev1alpha1.PodCliqueScalingGroupRunning))

			By("verifying PodGangSet reflects ScalingGroup status")
			helpers.ExpectPhaseEventually(ctx, k8sClient, podGangSet, string(grovecorev1alpha1.PodGangSetRunning))

			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return -1
				}
				return podGangSet.Status.ReadyReplicas
			}).Should(Equal(int32(3)))
		})
	})

	Context("When testing role-based startup coordination", func() {
		var podGangSet *grovecorev1alpha1.PodGangSet

		BeforeEach(func() {
			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-role-coordination").
				WithReplicas(1).
				Build()

			primaryClique := helpers.SimpleGangSetPodClique("primary", 1)
			primaryClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:        "leader",
				StartupType: ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
			}

			workerClique := helpers.SimpleGangSetPodClique("worker", 2)
			workerClique.Role = &grovecorev1alpha1.PodGangSetPodCliqueRole{
				Name:         "follower",
				StartupType:  ptr.To(grovecorev1alpha1.CliqueStartupTypeSequential),
				Dependencies: []string{"leader"},
			}

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				primaryClique,
				workerClique,
			}
		})

		It("should enforce startup dependencies", func() {
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

			By("verifying role assignments")
			podCliqueList := &grovecorev1alpha1.PodCliqueList{}
			err = k8sClient.List(ctx, podCliqueList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
			Expect(err).NotTo(HaveOccurred())

			roleMap := make(map[string]*grovecorev1alpha1.PodClique)
			for i, pc := range podCliqueList.Items {
				if pc.Spec.Role != nil {
					roleMap[pc.Spec.Role.Name] = &podCliqueList.Items[i]
				}
			}

			Expect(roleMap).To(HaveKey("leader"))
			Expect(roleMap).To(HaveKey("follower"))

			By("verifying dependency configuration")
			followerClique := roleMap["follower"]
			Expect(followerClique.Spec.Role.Dependencies).To(ContainElement("leader"))
		})

		It("should coordinate sequential startup", func() {
			By("creating the PodGangSet")
			err := k8sClient.Create(ctx, podGangSet)
			Expect(err).NotTo(HaveOccurred())

			By("getting created PodCliques")
			var leaderClique, followerClique *grovecorev1alpha1.PodClique
			Eventually(func() bool {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodGangSetName: podGangSet.Name})
				if err != nil || len(podCliqueList.Items) != 2 {
					return false
				}

				for i, pc := range podCliqueList.Items {
					if pc.Spec.Role != nil {
						if pc.Spec.Role.Name == "leader" {
							leaderClique = &podCliqueList.Items[i]
						} else if pc.Spec.Role.Name == "follower" {
							followerClique = &podCliqueList.Items[i]
						}
					}
				}
				return leaderClique != nil && followerClique != nil
			}).Should(BeTrue())

			By("simulating leader becoming ready first")
			leaderClique.Status.Phase = grovecorev1alpha1.PodCliqueRunning
			leaderClique.Status.ReadyReplicas = 1
			leaderClique.Status.Replicas = 1
			err = k8sClient.Status().Update(ctx, leaderClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying follower can now proceed")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(followerClique), followerClique)
				return err == nil
			}).Should(BeTrue())

			By("simulating follower becoming ready")
			followerClique.Status.Phase = grovecorev1alpha1.PodCliqueRunning
			followerClique.Status.ReadyReplicas = 2
			followerClique.Status.Replicas = 2
			err = k8sClient.Status().Update(ctx, followerClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PodGangSet coordination")
			helpers.ExpectPhaseEventually(ctx, k8sClient, podGangSet, string(grovecorev1alpha1.PodGangSetRunning))

			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return -1
				}
				return podGangSet.Status.ReadyReplicas
			}).Should(Equal(int32(3)))
		})
	})

	Context("When testing failure recovery across controllers", func() {
		var (
			podGangSet   *grovecorev1alpha1.PodGangSet
			scalingGroup *grovecorev1alpha1.PodCliqueScalingGroup
		)

		BeforeEach(func() {
			scalingGroup = helpers.NewPodCliqueScalingGroupBuilder().
				WithName("test-failure-sg").
				WithPodClique(helpers.SimpleScalingGroupPodClique("worker", 1)).
				Build()

			podGangSet = helpers.NewPodGangSetBuilder().
				WithName("test-failure-gang").
				WithReplicas(1).
				Build()

			podGangSet.Spec.PodCliques = []grovecorev1alpha1.PodGangSetPodClique{
				{
					Name: "sg-ref",
					ScalingGroupRef: &grovecorev1alpha1.PodGangSetScalingGroupRef{
						Name: scalingGroup.Name,
					},
				},
			}

			err := k8sClient.Create(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())
			err = k8sClient.Create(ctx, podGangSet)
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

		It("should handle ScalingGroup deletion gracefully", func() {
			By("deleting the ScalingGroup")
			err := k8sClient.Delete(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PodGangSet handles missing reference")
			helpers.ExpectConditionEventually(ctx, k8sClient, podGangSet,
				"Available", metav1.ConditionFalse, "")

			By("verifying PodGangSet enters degraded state")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podGangSet), podGangSet)
				if err != nil {
					return ""
				}
				return string(podGangSet.Status.Phase)
			}).Should(Equal(string(grovecorev1alpha1.PodGangSetDegraded)))
		})

		It("should recover when ScalingGroup is recreated", func() {
			By("deleting the ScalingGroup")
			err := k8sClient.Delete(ctx, scalingGroup)
			Expect(err).NotTo(HaveOccurred())

			helpers.ExpectResourceEventuallyDeleted(ctx, k8sClient, scalingGroup)

			By("recreating the ScalingGroup")
			newScalingGroup := helpers.NewPodCliqueScalingGroupBuilder().
				WithName(scalingGroup.Name).
				WithPodClique(helpers.SimpleScalingGroupPodClique("worker", 1)).
				Build()

			err = k8sClient.Create(ctx, newScalingGroup)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PodGangSet recovers")
			helpers.ExpectPhaseEventually(ctx, k8sClient, podGangSet, string(grovecorev1alpha1.PodGangSetPending))

			By("waiting for recovery to complete")
			Eventually(func() int {
				podCliqueList := &grovecorev1alpha1.PodCliqueList{}
				err := k8sClient.List(ctx, podCliqueList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueScalingGroupName: newScalingGroup.Name})
				if err != nil {
					return -1
				}
				return len(podCliqueList.Items)
			}).Should(Equal(1))
		})
	})
})
