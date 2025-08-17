package framework

import (
	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	"k8s.io/utils/ptr"
)

// defaultTestOperatorConfig returns the default operator configuration for tests
func defaultTestOperatorConfig() *configv1alpha1.OperatorConfiguration {
	return &configv1alpha1.OperatorConfiguration{
		Controllers: configv1alpha1.ControllerConfiguration{
			PodGangSet: configv1alpha1.PodGangSetControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
			PodClique: configv1alpha1.PodCliqueControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
			PodCliqueScalingGroup: configv1alpha1.PodCliqueScalingGroupControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
		},
	}
}
