package framework

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// installRBACWithRules installs RBAC resources with the given rules
func installRBACWithRules(ctx context.Context, client client.Client, namespace, serviceAccount string, rules []rbacv1.PolicyRule) error {
	// Create ServiceAccount
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceAccount,
			Namespace: namespace,
		},
	}
	if err := client.Create(ctx, sa); err != nil {
		return err
	}

	// Create ClusterRole
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccount + "-role",
		},
		Rules: rules,
	}
	if err := client.Create(ctx, cr); err != nil {
		return err
	}

	// Create ClusterRoleBinding
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: serviceAccount + "-binding",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccount,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     cr.Name,
		},
	}
	return client.Create(ctx, crb)
}

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
