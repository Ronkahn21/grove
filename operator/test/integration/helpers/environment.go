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
	"time"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var (
	testClient    client.Client
	testNamespace string
)

const (
	timeout  = time.Second * 30
	interval = time.Millisecond * 250
)

func SetTestEnvironment(client client.Client, namespace string) {
	testClient = client
	testNamespace = namespace
}

func GetTestClient() client.Client {
	return testClient
}

func GetTestNamespace() string {
	return testNamespace
}

func CleanupTestResources(ctx context.Context, client client.Client, namespace string) {
	By("deleting all PodGangSets")
	podGangSetList := &grovecorev1alpha1.PodGangSetList{}
	err := client.List(ctx, podGangSetList, client.InNamespace(namespace))
	if err == nil {
		for _, pgs := range podGangSetList.Items {
			err := client.Delete(ctx, &pgs)
			if err != nil && !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	By("deleting all PodCliques")
	podCliqueList := &grovecorev1alpha1.PodCliqueList{}
	err = client.List(ctx, podCliqueList, client.InNamespace(namespace))
	if err == nil {
		for _, pc := range podCliqueList.Items {
			err := client.Delete(ctx, &pc)
			if err != nil && !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	By("deleting all PodCliqueScalingGroups")
	scalingGroupList := &grovecorev1alpha1.PodCliqueScalingGroupList{}
	err = client.List(ctx, scalingGroupList, client.InNamespace(namespace))
	if err == nil {
		for _, sg := range scalingGroupList.Items {
			err := client.Delete(ctx, &sg)
			if err != nil && !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	By("deleting all Pods")
	podList := &corev1.PodList{}
	err = client.List(ctx, podList, client.InNamespace(namespace))
	if err == nil {
		for _, pod := range podList.Items {
			err := client.Delete(ctx, &pod)
			if err != nil && !apierrors.IsNotFound(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		}
	}

	By("waiting for resources to be deleted")
	Eventually(func() bool {
		podGangSetList := &grovecorev1alpha1.PodGangSetList{}
		err := client.List(ctx, podGangSetList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		podCliqueList := &grovecorev1alpha1.PodCliqueList{}
		err = client.List(ctx, podCliqueList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		scalingGroupList := &grovecorev1alpha1.PodCliqueScalingGroupList{}
		err = client.List(ctx, scalingGroupList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		podList := &corev1.PodList{}
		err = client.List(ctx, podList, client.InNamespace(namespace))
		if err != nil {
			return false
		}

		return len(podGangSetList.Items) == 0 &&
			len(podCliqueList.Items) == 0 &&
			len(scalingGroupList.Items) == 0 &&
			len(podList.Items) == 0
	}, timeout, interval).Should(BeTrue())
}

func WaitForResourceDeletion(ctx context.Context, client client.Client, obj client.Object) {
	Eventually(func() bool {
		key := types.NamespacedName{Name: obj.GetName(), Namespace: obj.GetNamespace()}
		err := client.Get(ctx, key, obj)
		return apierrors.IsNotFound(err)
	}, timeout, interval).Should(BeTrue())
}
