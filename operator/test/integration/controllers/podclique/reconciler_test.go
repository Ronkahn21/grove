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

package podclique

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	"github.com/NVIDIA/grove/operator/test/integration/helpers"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("PodClique Controller", func() {
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

	Context("When creating a simple PodClique", func() {
		var podClique *grovecorev1alpha1.PodClique

		BeforeEach(func() {
			podClique = helpers.NewPodCliqueBuilder().
				WithName("test-simple-pc").
				WithReplicas(2).
				Build()
		})

		It("should create the PodClique successfully", func() {
			By("creating the PodClique")
			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the PodClique exists")
			helpers.ExpectResourceEventuallyExists(ctx, k8sClient, podClique)

			By("checking initial status")
			Eventually(func() string {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
				if err != nil {
					return ""
				}
				return string(podClique.Status.Phase)
			}).Should(Equal(string(grovecorev1alpha1.PodCliquePending)))
		})

		It("should create pods according to replica count", func() {
			By("creating the PodClique")
			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pods to be created")
			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 2)

			By("verifying pod properties")
			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(podList.Items).To(HaveLen(2))

			for _, pod := range podList.Items {
				Expect(pod.Spec.Containers).To(HaveLen(1))
				Expect(pod.Spec.Containers[0].Name).To(Equal("test-container"))
				Expect(pod.Spec.Containers[0].Image).To(Equal("nginx:latest"))
			}
		})

		It("should update status when pods become ready", func() {
			By("creating the PodClique")
			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pods to be created")
			var createdPods []corev1.Pod
			Eventually(func() bool {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
				if err != nil || len(podList.Items) != 2 {
					return false
				}
				createdPods = podList.Items
				return true
			}).Should(BeTrue())

			By("simulating pods becoming ready")
			for i := range createdPods {
				pod := &createdPods[i]
				pod.Status.Phase = corev1.PodRunning
				pod.Status.Conditions = []corev1.PodCondition{
					{
						Type:   corev1.PodReady,
						Status: corev1.ConditionTrue,
					},
				}
				err = k8sClient.Status().Update(ctx, pod)
				Expect(err).NotTo(HaveOccurred())
			}

			By("verifying PodClique status updates")
			helpers.ExpectPhaseEventually(ctx, k8sClient, podClique, string(grovecorev1alpha1.PodCliqueRunning))

			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
				if err != nil {
					return -1
				}
				return podClique.Status.ReadyReplicas
			}).Should(Equal(int32(2)))
		})
	})

	Context("When scaling a PodClique", func() {
		var podClique *grovecorev1alpha1.PodClique

		BeforeEach(func() {
			podClique = helpers.NewPodCliqueBuilder().
				WithName("test-scaling-pc").
				WithReplicas(1).
				Build()

			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 1)
		})

		It("should scale up when replicas are increased", func() {
			By("updating replica count")
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
			Expect(err).NotTo(HaveOccurred())

			podClique.Spec.Replicas = ptr.To(int32(3))
			err = k8sClient.Update(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for additional pods to be created")
			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 3)

			By("verifying status reflects new replica count")
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
				if err != nil {
					return -1
				}
				return podClique.Status.Replicas
			}).Should(Equal(int32(3)))
		})

		It("should scale down when replicas are decreased", func() {
			By("scaling up first")
			err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
			Expect(err).NotTo(HaveOccurred())

			podClique.Spec.Replicas = ptr.To(int32(3))
			err = k8sClient.Update(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 3)

			By("scaling down")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
			Expect(err).NotTo(HaveOccurred())

			podClique.Spec.Replicas = ptr.To(int32(1))
			err = k8sClient.Update(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pods to be deleted")
			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 1)

			By("verifying status reflects new replica count")
			Eventually(func() int32 {
				err := k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
				if err != nil {
					return -1
				}
				return podClique.Status.Replicas
			}).Should(Equal(int32(1)))
		})
	})

	Context("When creating a PodClique with roles", func() {
		var podClique *grovecorev1alpha1.PodClique

		BeforeEach(func() {
			podClique = helpers.NewPodCliqueBuilder().
				WithName("test-role-pc").
				WithReplicas(1).
				WithRole("worker").
				WithStartupType(grovecorev1alpha1.CliqueStartupTypeParallel).
				Build()
		})

		It("should create PodClique with role configuration", func() {
			By("creating the PodClique")
			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("verifying role configuration")
			err = k8sClient.Get(ctx, client.ObjectKeyFromObject(podClique), podClique)
			Expect(err).NotTo(HaveOccurred())

			Expect(podClique.Spec.Role).NotTo(BeNil())
			Expect(podClique.Spec.Role.Name).To(Equal("worker"))
			Expect(podClique.Spec.Role.StartupType).NotTo(BeNil())
			Expect(*podClique.Spec.Role.StartupType).To(Equal(grovecorev1alpha1.CliqueStartupTypeParallel))

			By("verifying pods have role labels")
			Eventually(func() bool {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
				if err != nil || len(podList.Items) != 1 {
					return false
				}

				pod := &podList.Items[0]
				return pod.Labels[common.LabelPodCliqueRole] == "worker"
			}).Should(BeTrue())
		})
	})

	Context("When deleting a PodClique", func() {
		var podClique *grovecorev1alpha1.PodClique

		BeforeEach(func() {
			podClique = helpers.NewPodCliqueBuilder().
				WithName("test-delete-pc").
				WithReplicas(2).
				Build()

			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 2)
		})

		It("should delete all associated pods", func() {
			By("deleting the PodClique")
			err := k8sClient.Delete(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for PodClique to be deleted")
			helpers.ExpectResourceEventuallyDeleted(ctx, k8sClient, podClique)

			By("verifying all pods are deleted")
			Eventually(func() int {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
				if err != nil {
					return -1
				}
				return len(podList.Items)
			}).Should(Equal(0))
		})
	})

	Context("When handling pod failures", func() {
		var podClique *grovecorev1alpha1.PodClique

		BeforeEach(func() {
			podClique = helpers.NewPodCliqueBuilder().
				WithName("test-failure-pc").
				WithReplicas(2).
				Build()

			err := k8sClient.Create(ctx, podClique)
			Expect(err).NotTo(HaveOccurred())

			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 2)
		})

		It("should recreate failed pods", func() {
			By("getting one of the pods")
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
			Expect(err).NotTo(HaveOccurred())
			Expect(podList.Items).To(HaveLen(2))

			podToDelete := &podList.Items[0]
			originalUID := podToDelete.UID

			By("deleting a pod to simulate failure")
			err = k8sClient.Delete(ctx, podToDelete)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for pod to be recreated")
			Eventually(func() bool {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{common.LabelPodCliqueName: podClique.Name})
				if err != nil || len(podList.Items) != 2 {
					return false
				}

				for _, pod := range podList.Items {
					if pod.UID == originalUID {
						return false
					}
				}
				return true
			}).Should(BeTrue())

			By("verifying PodClique maintains desired replica count")
			helpers.ExpectPodsWithLabel(ctx, k8sClient, namespace,
				common.LabelPodCliqueName, podClique.Name, 2)
		})
	})
})
