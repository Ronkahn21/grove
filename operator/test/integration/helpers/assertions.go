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
	"context"
	"fmt"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func ExpectReconcileSuccess(ctx context.Context, reconciler reconcile.Reconciler, req reconcile.Request) {
	result, err := reconciler.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Requeue).To(BeFalse())
}

func ExpectReconcileRequeue(ctx context.Context, reconciler reconcile.Reconciler, req reconcile.Request) {
	result, err := reconciler.Reconcile(ctx, req)
	Expect(err).NotTo(HaveOccurred())
	Expect(result.Requeue || result.RequeueAfter > 0).To(BeTrue())
}

func ExpectResourceExists(ctx context.Context, client client.Client, obj client.Object) {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	err := client.Get(ctx, key, obj)
	Expect(err).NotTo(HaveOccurred())
}

func ExpectResourceNotFound(ctx context.Context, client client.Client, obj client.Object) {
	key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
	err := client.Get(ctx, key, obj)
	Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func ExpectStatusCondition(obj client.Object, conditionType string, status metav1.ConditionStatus, reason string) {
	switch v := obj.(type) {
	case *grovecorev1alpha1.PodClique:
		Expect(v).To(HaveCondition(conditionType, status, reason))
	case *grovecorev1alpha1.PodCliqueScalingGroup:
		Expect(v).To(HaveCondition(conditionType, status, reason))
	default:
		Fail("Unsupported object type for condition check")
	}
}

func ExpectReadyPods(obj client.Object, count int32) {
	switch v := obj.(type) {
	case *grovecorev1alpha1.PodClique:
		Expect(v).To(HaveReadyPods(count))
	case *grovecorev1alpha1.PodCliqueScalingGroup:
		Expect(v).To(HaveReadyPods(count))
	default:
		Fail("Unsupported object type for ready pods check")
	}
}

func ExpectPodsCreated(ctx context.Context, client client.Client, namespace string, expectedCount int) {
	Eventually(func() int {
		podList := &corev1.PodList{}
		err := client.List(ctx, podList, client.InNamespace(namespace))
		if err != nil {
			return -1
		}
		return len(podList.Items)
	}, timeout, interval).Should(Equal(expectedCount))
}

func ExpectPodsWithLabel(ctx context.Context, client client.Client, namespace, labelKey, labelValue string, expectedCount int) {
	Eventually(func() int {
		podList := &corev1.PodList{}
		labelSelector := map[string]string{labelKey: labelValue}
		err := client.List(ctx, podList,
			client.InNamespace(namespace),
			client.MatchingLabels(labelSelector))
		if err != nil {
			return -1
		}
		return len(podList.Items)
	}, timeout, interval).Should(Equal(expectedCount))
}

func ExpectResourceEventuallyExists(ctx context.Context, client client.Client, obj client.Object) {
	Eventually(func() error {
		key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		return client.Get(ctx, key, obj)
	}, timeout, interval).Should(Succeed())
}

func ExpectResourceEventuallyDeleted(ctx context.Context, client client.Client, obj client.Object) {
	Eventually(func() bool {
		key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		err := client.Get(ctx, key, obj)
		return apierrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}

func ExpectConditionEventually(ctx context.Context, client client.Client, obj client.Object, conditionType string, status metav1.ConditionStatus, reason string) {
	Eventually(func() error {
		key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		err := client.Get(ctx, key, obj)
		if err != nil {
			return err
		}

		conditions, err := getConditions(obj)
		if err != nil {
			return err
		}

		for _, condition := range conditions {
			if condition.Type == conditionType &&
				condition.Status == status &&
				(reason == "" || condition.Reason == reason) {
				return nil
			}
		}

		return fmt.Errorf("condition %s=%s with reason=%s not found", conditionType, status, reason)
	}, timeout, interval).Should(Succeed())
}
