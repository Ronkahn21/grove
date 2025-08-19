package framework

import (
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// ManagerOption is a functional option for configuring the test manager
type ManagerOption func(*ManagerOptions)

// ManagerOptions holds configuration options for the test manager
type ManagerOptions struct {
	Scheme          *runtime.Scheme
	ConcurrentSyncs int32
	WebhookServer   webhook.Server
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

// WithWebhookServer sets the webhook server for the manager
func WithWebhookServer(server webhook.Server) ManagerOption {
	return func(o *ManagerOptions) {
		o.WebhookServer = server
	}
}
