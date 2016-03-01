// Package provider provides different implementations for compute providers.
package provider

// A Provider implements the methods necessary to manage Instances on a given
// compute provider.
type Provider interface {
	List() ([]Instance, error)
	Create(image string) (Instance, error)
	Get(providerID string) (Instance, error)
	Destroy(providerID string) error
}

// An Instance is a single compute instance
type Instance struct {
	ID        string
	State     InstanceState
	IPAddress string
}

// An InstanceState is the state an instance can be in. Valid values are the
// InstanceState... constants defined in this package.
type InstanceState string

const (
	// InstanceStateStarting is the state of an instance that is starting up,
	// but not yet ready to be connected to.
	InstanceStateStarting InstanceState = "starting"

	// InstanceStateRunning is the state of an instance that has finished
	// starting up, and will remain in this state until being told to terminate.
	InstanceStateRunning InstanceState = "running"

	// InstanceStateTerminating is the state of an instance that has been told
	// to terminate, but is not yet finished doing that.
	InstanceStateTerminating InstanceState = "terminating"
)
