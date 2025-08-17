package framework

import (
	groveclient "github.com/NVIDIA/grove/operator/internal/client"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// TestManager wraps a controller-runtime manager for testing
type TestManager struct {
	Manager manager.Manager
	Config  *rest.Config
}

// CreateTestManager creates a new test manager with the given configuration
func CreateTestManager(cfg *rest.Config, opts ...ManagerOption) (*TestManager, error) {
	// Apply default options with Grove's scheme
	options := &ManagerOptions{
		Scheme: groveclient.Scheme,
	}

	// Apply provided options
	for _, opt := range opts {
		opt(options)
	}

	// Create manager with test-specific options
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 options.Scheme,
		LeaderElection:         false, // Disable leader election in tests
		HealthProbeBindAddress: "0",   // Disable health probes in tests
	})
	if err != nil {
		return nil, err
	}

	return &TestManager{
		Manager: mgr,
		Config:  cfg,
	}, nil
}
