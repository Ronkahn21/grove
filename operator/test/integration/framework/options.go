package framework

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// ManagerOption is a functional option for configuring the test manager
type ManagerOption func(*ManagerOptions)

// ManagerOptions holds configuration options for the test manager
type ManagerOptions struct {
	Scheme          *runtime.Scheme
	ConcurrentSyncs int32
}

// WithConcurrentSyncs sets the number of concurrent syncs for controllers
func WithConcurrentSyncs(syncs int32) ManagerOption {
	return func(o *ManagerOptions) {
		o.ConcurrentSyncs = syncs
	}
}

// WithScheme sets the runtime scheme for the manager
func WithScheme(scheme *runtime.Scheme) ManagerOption {
	return func(o *ManagerOptions) {
		o.Scheme = scheme
	}
}
