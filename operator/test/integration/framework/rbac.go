package framework

import (
	rbacv1 "k8s.io/api/rbac/v1"
)

// getOperatorPolicyRules returns the default policy rules for the operator
func getOperatorPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "services", "endpoints", "persistentvolumeclaims", "events", "configmaps", "secrets"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"grove.io"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"scheduler.grove.io"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}
}
