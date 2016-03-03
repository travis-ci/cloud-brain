// Package cloud provides different implementations for cloud providers.
package cloud

// A Provider implements the methods necessary to manage Instances on a given
// cloud provider.
type Provider interface {
	Name() string
	List() ([]Instance, error)
	Create(attr CreateAttributes) (Instance, error)
	Get(providerID string) (Instance, error)
	Destroy(providerID string) error
}

// An Instance is a single compute instance
type Instance struct {
	ID        string
	State     InstanceState
	IPAddress string
}

// CreateAttributes contains the attributes needed to start an instance.
type CreateAttributes struct {
	ImageName    string
	InstanceType InstanceType
	PublicSSHKey string
}

// An InstanceState is the state an instance can be in. Valid values are the
// InstanceState… constants defined in this package.
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

// An InstanceType is the type of instance to start. Valid values are the
// InstanceType… constants defined in this package. The instance type defines
// things such as the amount of resources available.
type InstanceType string

const (
	// InstanceTypeStandard is the default instance type.
	InstanceTypeStandard InstanceType = "standard"

	// InstanceTypePremium is an instance type with more resources available
	// than the standard instance type.
	InstanceTypePremium InstanceType = "premium"
)
