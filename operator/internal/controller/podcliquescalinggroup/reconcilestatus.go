// /*
// Copyright 2025 The Grove Authors.
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
	"fmt"

	grovecorev1alpha1 "github.com/NVIDIA/grove/operator/api/core/v1alpha1"
	"github.com/NVIDIA/grove/operator/internal/common"
	componentutils "github.com/NVIDIA/grove/operator/internal/component/utils"
	ctrlcommon "github.com/NVIDIA/grove/operator/internal/controller/common"
	k8sutils "github.com/NVIDIA/grove/operator/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) reconcileStatus(ctx context.Context, logger logr.Logger, pcsg *grovecorev1alpha1.PodCliqueScalingGroup) ctrlcommon.ReconcileStepResult {
	pgs, err := componentutils.GetOwnerPodGangSet(ctx, r.client, pcsg.ObjectMeta)
	if err != nil {
		return ctrlcommon.ReconcileWithErrors("failed to get owner PodGangSet", err)
	}

	pclqsPerPCSGReplica, err := r.getPodCliquesPerPCSGReplica(ctx, pgs.Name, client.ObjectKeyFromObject(pcsg))
	if err != nil {
		return ctrlcommon.ReconcileWithErrors(fmt.Sprintf("failed to list PodCliques for PodCliqueScalingGroup: %q", client.ObjectKeyFromObject(pcsg)), err)
	}

	if pcsg.Status.ObservedGeneration != nil {
		mutateReplicas(logger, pcsg, pclqsPerPCSGReplica)
		mutateMinAvailableBreachedCondition(logger, pcsg, pclqsPerPCSGReplica)
	availableReplicas, err := r.computePCSGAvailability(ctx, logger, pgs.Name, pcsg)
	if err != nil {
		logger.Error(err, "failed to calculate available replicas")
		return ctrlcommon.ReconcileWithErrors("failed to calculate available replicas", err)
	}

	r.mutateMinAvailableBreachedCondition(pcsg, availableReplicas)

	r.mutateAvailableReplica(pcsg, availableReplicas)

	if err = mutateSelector(pgs, pcsg); err != nil {
		logger.Error(err, "failed to update selector for PodCliqueScalingGroup")
		return ctrlcommon.ReconcileWithErrors("failed to update selector for PodCliqueScalingGroup", err)
	}

	if err = r.client.Status().Update(ctx, pcsg); err != nil {
		return ctrlcommon.ReconcileWithErrors("failed to update the status with label selector and replicas", err)
	}

	return ctrlcommon.ContinueReconcile()
}

func mutateReplicas(logger logr.Logger, pcsg *grovecorev1alpha1.PodCliqueScalingGroup, pclqsPerPCSGReplica map[string][]grovecorev1alpha1.PodClique) {
	pcsg.Status.Replicas = pcsg.Spec.Replicas
	var scheduledReplicas int32
	for pcsgReplicaIndex, pclqs := range pclqsPerPCSGReplica {
		isScheduled := lo.Reduce(pclqs, func(agg bool, pclq grovecorev1alpha1.PodClique, _ int) bool {
			return agg && k8sutils.IsConditionTrue(pclq.Status.Conditions, grovecorev1alpha1.ConditionTypePodCliqueScheduled)
		}, true)
		if isScheduled {
			scheduledReplicas++
		}
		logger.Info("PodCliqueScalingGroup replica scheduled status", "pcsgReplicaIndex", pcsgReplicaIndex, "isScheduled", isScheduled)
	}
	pcsg.Status.ScheduledReplicas = scheduledReplicas
}

func mutateMinAvailableBreachedCondition(logger logr.Logger, pcsg *grovecorev1alpha1.PodCliqueScalingGroup, pclqsPerPCSGReplica map[string][]grovecorev1alpha1.PodClique) {
	newCondition := computeMinAvailableBreachedCondition(logger, pcsg, pclqsPerPCSGReplica)
	if k8sutils.HasConditionChanged(pcsg.Status.Conditions, newCondition) {
		meta.SetStatusCondition(&pcsg.Status.Conditions, newCondition)
	}
}

func computeMinAvailableBreachedCondition(logger logr.Logger, pcsg *grovecorev1alpha1.PodCliqueScalingGroup, pclqsPerPCSGReplica map[string][]grovecorev1alpha1.PodClique) metav1.Condition {
	minAvailable := int(*pcsg.Spec.MinAvailable)
	scheduledReplicas := int(pcsg.Status.ScheduledReplicas)
	if scheduledReplicas < minAvailable {
		return metav1.Condition{
			Type:    grovecorev1alpha1.ConditionTypeMinAvailableBreached,
			Status:  metav1.ConditionFalse,
			Reason:  grovecorev1alpha1.ConditionReasonInsufficientScheduledPCSGReplicas,
			Message: fmt.Sprintf("Insufficient scheduled replicas. expected at least: %d, found: %d", minAvailable, scheduledReplicas),
		}
	}
	minAvailableBreachedReplicas := computeMinAvailableBreachedReplicas(logger, pclqsPerPCSGReplica)
	readyReplicas := scheduledReplicas - minAvailableBreachedReplicas
	if readyReplicas < minAvailable {
		return metav1.Condition{
			Type:    grovecorev1alpha1.ConditionTypeMinAvailableBreached,
			Status:  metav1.ConditionTrue,
			Reason:  grovecorev1alpha1.ConditionReasonInsufficientReadyPCSGReplicas,
			Message: fmt.Sprintf("Insufficient PodCliqueScalingGroup ready replicas, expected at least: %d, found: %d", minAvailable, readyReplicas),
		}
	}
	return metav1.Condition{
		Type:    grovecorev1alpha1.ConditionTypeMinAvailableBreached,
		Status:  metav1.ConditionFalse,
		Reason:  grovecorev1alpha1.ConditionReasonSufficientReadyPCSGReplicas,
		Message: fmt.Sprintf("Sufficient PodCliqueScalingGroup ready replicas, expected at least: %d, found: %d", minAvailable, readyReplicas),
func (r *Reconciler) mutateAvailableReplica(pcsg *grovecorev1alpha1.PodCliqueScalingGroup, availableReplicas int32) {
	pcsg.Status.AvailableReplicas = availableReplicas
}

func (r *Reconciler) mutateMinAvailableBreachedCondition(pcsg *grovecorev1alpha1.PodCliqueScalingGroup, availableReplicas int32) {
	newCondition := r.computeMinAvailableBreachedCondition(availableReplicas, pcsg)
	if k8sutils.HasConditionChanged(pcsg.Status.Conditions, *newCondition) {
		meta.SetStatusCondition(&pcsg.Status.Conditions, *newCondition)
	}
}

func (r *Reconciler) computeMinAvailableBreachedCondition(availableReplicas int32, pcsg *grovecorev1alpha1.PodCliqueScalingGroup) *metav1.Condition {
	if availableReplicas < ptr.Deref(pcsg.Spec.MinAvailable, defaultPCSGMinAvailable) {
		return &metav1.Condition{
			Type:    grovecorev1alpha1.ConditionTypeMinAvailableBreached,
			Status:  metav1.ConditionTrue,
			Reason:  grovecorev1alpha1.ConditionReasonInsufficientReadyPCSGReplicas,
			Message: fmt.Sprintf("Insufficient PodCliqueScalingGroup replicas, expected at least: %d, found: %d", defaultPCSGMinAvailable, availableReplicas),
		}
	}
	return &metav1.Condition{
		Type:    grovecorev1alpha1.ConditionTypeMinAvailableBreached,
		Status:  metav1.ConditionFalse,
		Reason:  grovecorev1alpha1.ConditionReasonSufficientReadyPCSGReplicas,
		Message: fmt.Sprintf("Sufficient PodCliqueScalingGroup replicas, expected at least: %d, found: %d", defaultPCSGMinAvailable, availableReplicas),
	}
}

func (r *Reconciler) computePCSGAvailability(ctx context.Context, logger logr.Logger, pgsName string, pcsg *grovecorev1alpha1.PodCliqueScalingGroup) (int32, error) {
func computeMinAvailableBreachedReplicas(logger logr.Logger, pclqsPerPCSGReplica map[string][]grovecorev1alpha1.PodClique) int {
	var breachedReplicas int
	for pcsgReplicaIndex, pclqs := range pclqsPerPCSGReplica {
		isMinAvailableBreached := lo.Reduce(pclqs, func(agg bool, pclq grovecorev1alpha1.PodClique, _ int) bool {
			return agg || k8sutils.IsConditionTrue(pclq.Status.Conditions, grovecorev1alpha1.ConditionTypeMinAvailableBreached)
		}, false)
		if isMinAvailableBreached {
			breachedReplicas++
		}
		logger.Info("PodCliqueScalingGroup replica has MinAvailableBreached condition set to true", "pcsgReplicaIndex", pcsgReplicaIndex, "isMinAvailableBreached", isMinAvailableBreached)
	}
	return breachedReplicas
}

func (r *Reconciler) getPodCliquesPerPCSGReplica(ctx context.Context, pgsName string, pcsgObjKey client.ObjectKey) (map[string][]grovecorev1alpha1.PodClique, error) {
	selectorLabels := lo.Assign(
		k8sutils.GetDefaultLabelsForPodGangSetManagedResources(pgsName),
		map[string]string{
			grovecorev1alpha1.LabelPodCliqueScalingGroup: pcsgObjKey.Name,
			grovecorev1alpha1.LabelComponentKey:          common.NamePCSGPodClique,
		},
	)
	pclqs, err := componentutils.GetPCLQsByOwner(ctx,
		r.client,
		grovecorev1alpha1.PodCliqueScalingGroupKind,
		pcsgObjKey,
		selectorLabels,
	)
	if err != nil {
		return 0, err
	}
	pclqsPerPCSGReplica := componentutils.GroupPCLQsByPCSGReplicaIndex(pclqs)
	return pclqsPerPCSGReplica, nil

	// group PodCliques per PodCliqueScalingGroup replica
	pcsgReplicaPCLQs := componentutils.GroupPCLQsByPCSGReplicaIndex(pcsgPCLQs)
	availableReplicas := int32(0)

	for pcsgReplicaIndex, pclqs := range pcsgReplicaPCLQs {
		pcsgReplicaAvailable := checkReplicaAvailability(logger, pcsgReplicaIndex, pclqs)
		// A PCSG replica is available when all constituent PodCliques have MinAvailableBreached = False
		if pcsgReplicaAvailable {
			availableReplicas++
		}
	}

	logger.Info("Calculated available replicas for PCSG", "pcsg", client.ObjectKeyFromObject(pcsg), "availableReplicas", availableReplicas, "totalReplicas", pcsg.Spec.Replicas)
	return availableReplicas, nil
}

func checkReplicaAvailability(logger logr.Logger, pcsgReplicaIndex string, pclqs []grovecorev1alpha1.PodClique) bool {
	for _, pclq := range pclqs {
		logger.Info("MinAvailable condition status for PCLQ", "pclq", client.ObjectKeyFromObject(&pclq), "status", k8sutils.GetConditionStatus(pclq.Status.Conditions, grovecorev1alpha1.ConditionTypeMinAvailableBreached))
		if k8sutils.IsResourceTerminating(pclq.ObjectMeta) {
			logger.Info("PCLQ is marked for termination, PCSG replica consider not available",
				"pcsgReplicaIndex", pcsgReplicaIndex, "pclq", client.ObjectKeyFromObject(&pclq))
			return false
		}
		if k8sutils.IsConditionTrue(pclq.Status.Conditions, grovecorev1alpha1.ConditionTypeMinAvailableBreached) {
			logger.Info("PCLQ has MinAvailableBreached condition set to true,"+
				" PCSG replica consider not available", "pcsgReplicaIndex",
				pcsgReplicaIndex, "pclq", client.ObjectKeyFromObject(&pclq))
			return false
		}
	}
	logger.Info("PCSG replica is available", "pcsgReplicaIndex", pcsgReplicaIndex)
	return true
}

func mutateSelector(pgs *grovecorev1alpha1.PodGangSet, pcsg *grovecorev1alpha1.PodCliqueScalingGroup) error {
	pgsReplicaIndex, err := k8sutils.GetPodGangSetReplicaIndex(pcsg.ObjectMeta)
	if err != nil {
		return err
	}
	matchingPCSGConfig, ok := lo.Find(pgs.Spec.Template.PodCliqueScalingGroupConfigs, func(pcsgConfig grovecorev1alpha1.PodCliqueScalingGroupConfig) bool {
		pcsgFQN := grovecorev1alpha1.GeneratePodCliqueScalingGroupName(grovecorev1alpha1.ResourceNameReplica{Name: pgs.Name, Replica: pgsReplicaIndex}, pcsgConfig.Name)
		return pcsgFQN == pcsg.Name
	})
	if !ok {
		// This should ideally never happen but if you find a PCSG that is not defined in PGS then just ignore it.
		return nil
	}
	// No ScaleConfig has been defined of this PCSG, therefore there is no need to add a selector in the status.
	if matchingPCSGConfig.ScaleConfig == nil {
		return nil
	}
	labels := lo.Assign(
		k8sutils.GetDefaultLabelsForPodGangSetManagedResources(pgs.Name),
		map[string]string{
			grovecorev1alpha1.LabelPodCliqueScalingGroup: pcsg.Name,
		},
	)
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{MatchLabels: labels})
	if err != nil {
		return fmt.Errorf("%w: failed to create label selector for PodCliqueScalingGroup %v", err, client.ObjectKeyFromObject(pcsg))
	}
	pcsg.Status.Selector = ptr.To(selector.String())
	return nil
}
