package framework

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// TestEnv represents a built test environment
type TestEnv struct {
	T          *testing.T
	Client     client.Client
	Manager    manager.Manager
	Ctx        context.Context
	Namespaces []string
	Objects    []client.Object
}

// GetNamespace returns the first created namespace (for simple tests)
func (te *TestEnv) GetNamespace() string {
	if len(te.Namespaces) > 0 {
		return te.Namespaces[0]
	}
	return ""
}

// GetNamespaceByIndex returns a namespace by index
func (te *TestEnv) GetNamespaceByIndex(i int) string {
	if i < len(te.Namespaces) {
		return te.Namespaces[i]
	}
	return ""
}

// CreateNamespace creates an additional namespace
func (te *TestEnv) CreateNamespace(name string) string {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	if err := te.Client.Create(te.Ctx, ns); err != nil {
		te.T.Fatalf("failed to create namespace: %v", err)
	}

	te.T.Cleanup(func() {
		_ = te.Client.Delete(context.Background(), ns)
	})

	return name
}
