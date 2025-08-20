package framework

import (
	configv1alpha1 "github.com/NVIDIA/grove/operator/api/config/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
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

func defaultWebhookRules() []admissionregistrationv1.RuleWithOperations {
	return []admissionregistrationv1.RuleWithOperations{
		{
			Operations: []admissionregistrationv1.OperationType{
				admissionregistrationv1.Create,
				admissionregistrationv1.Update,
			},
			Rule: admissionregistrationv1.Rule{
				APIGroups:   []string{"grove.io"},
				APIVersions: []string{"v1alpha1"},
				Resources:   []string{"podgangsets"},
			},
		},
	}
}
