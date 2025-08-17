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
	"fmt"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/onsi/gomega/format"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type HaveConditionMatcher struct {
	conditionType string
	status        metav1.ConditionStatus
	reason        string
	message       string
}

func HaveCondition(conditionType string, status metav1.ConditionStatus, reason string) types.GomegaMatcher {
	return &HaveConditionMatcher{
		conditionType: conditionType,
		status:        status,
		reason:        reason,
	}
}

func HaveConditionWithMessage(conditionType string, status metav1.ConditionStatus, reason, message string) types.GomegaMatcher {
	return &HaveConditionMatcher{
		conditionType: conditionType,
		status:        status,
		reason:        reason,
		message:       message,
	}
}

func (matcher *HaveConditionMatcher) Match(actual interface{}) (success bool, err error) {
	conditions, err := getConditions(actual)
	if err != nil {
		return false, err
	}

	for _, condition := range conditions {
		if condition.Type == matcher.conditionType &&
			condition.Status == matcher.status &&
			(matcher.reason == "" || condition.Reason == matcher.reason) &&
			(matcher.message == "" || condition.Message == matcher.message) {
			return true, nil
		}
	}
	return false, nil
}

func (matcher *HaveConditionMatcher) FailureMessage(actual interface{}) (message string) {
	conditions, _ := getConditions(actual)
	return fmt.Sprintf("Expected\n\t%s\nto have condition %s=%s with reason=%s, but got conditions:\n\t%s",
		format.Object(actual, 1),
		matcher.conditionType,
		matcher.status,
		matcher.reason,
		format.Object(conditions, 1))
}

func (matcher *HaveConditionMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nnot to have condition %s=%s with reason=%s",
		format.Object(actual, 1),
		matcher.conditionType,
		matcher.status,
		matcher.reason)
}

type HaveReadyPodsMatcher struct {
	count int32
}

func HaveReadyPods(count int32) types.GomegaMatcher {
	return &HaveReadyPodsMatcher{
		count: count,
	}
}

func (matcher *HaveReadyPodsMatcher) Match(actual interface{}) (success bool, err error) {
	readyCount, err := getReadyPods(actual)
	if err != nil {
		return false, err
	}
	return readyCount == matcher.count, nil
}

func (matcher *HaveReadyPodsMatcher) FailureMessage(actual interface{}) (message string) {
	readyCount, _ := getReadyPods(actual)
	return fmt.Sprintf("Expected\n\t%s\nto have %d ready pods, but had %d ready pods",
		format.Object(actual, 1),
		matcher.count,
		readyCount)
}

func (matcher *HaveReadyPodsMatcher) NegatedFailureMessage(actual interface{}) (message string) {
	return fmt.Sprintf("Expected\n\t%s\nnot to have %d ready pods",
		format.Object(actual, 1),
		matcher.count)
}

func getConditions(obj interface{}) ([]metav1.Condition, error) {
	switch v := obj.(type) {
	case *grovecorev1alpha1.PodClique:
		return v.Status.Conditions, nil
	case *grovecorev1alpha1.PodCliqueScalingGroup:
		return v.Status.Conditions, nil
	default:
		return nil, fmt.Errorf("object type %T does not have conditions", obj)
	}
}

func getReadyPods(obj interface{}) (int32, error) {
	switch v := obj.(type) {
	case *grovecorev1alpha1.PodClique:
		return v.Status.ReadyReplicas, nil
	case *grovecorev1alpha1.PodCliqueScalingGroup:
		return v.Status.ReadyReplicas, nil
	default:
		return 0, fmt.Errorf("object type %T does not have ready pods count", obj)
	}
}
