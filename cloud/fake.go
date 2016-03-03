package cloud

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
)

// FakeProvider is an in-memory Provider suitable for tests.
type FakeProvider struct {
	instancesMutex sync.Mutex
	instances      map[string]Instance
	counter        uint64
}

// MarkRunning marks a VM as running and gives it a random IP address.
func (p *FakeProvider) MarkRunning(id string) {
	p.instancesMutex.Lock()
	defer p.instancesMutex.Unlock()

	inst := p.instances[id]

	ipAddress := make([]byte, 4)
	rand.Read(ipAddress)
	inst.IPAddress = fmt.Sprintf("%d.%d.%d.%d", ipAddress[0], ipAddress[1], ipAddress[2], ipAddress[3])
	inst.State = InstanceStateRunning
	p.instances[inst.ID] = inst
}

func (p *FakeProvider) Name() string {
	return "fake"
}

// List returns all the instances in the fake provider.
func (p *FakeProvider) List() ([]Instance, error) {
	if rand.Intn(10) == 0 {
		return nil, fmt.Errorf("random error occurred")
	}

	p.instancesMutex.Lock()
	defer p.instancesMutex.Unlock()

	var instances []Instance
	for _, instance := range p.instances {
		instances = append(instances, instance)
	}

	return instances, nil
}

// Create creates an instance in the fake provider.
func (p *FakeProvider) Create(image string) (Instance, error) {
	if rand.Intn(5) == 0 {
		return Instance{}, fmt.Errorf("random error occurred")
	}

	p.instancesMutex.Lock()
	defer p.instancesMutex.Unlock()

	if image == "" {
		return Instance{}, fmt.Errorf("image is required")
	}

	if image == "standard-image" {
		count := atomic.AddUint64(&p.counter, 1)
		inst := Instance{
			ID:    fmt.Sprintf("instance-standard-image-%d", count),
			State: InstanceStateStarting,
		}
		if p.instances == nil {
			p.instances = make(map[string]Instance)
		}
		p.instances[inst.ID] = inst

		return inst, nil
	}

	return Instance{}, fmt.Errorf("unknown image")
}

func (p *FakeProvider) Get(id string) (Instance, error) {
	p.instancesMutex.Lock()
	defer p.instancesMutex.Unlock()

	instance, ok := p.instances[id]
	if !ok {
		return Instance{}, fmt.Errorf("instance not found")
	}

	return instance, nil
}

func (p *FakeProvider) Destroy(id string) error {
	p.instancesMutex.Lock()
	defer p.instancesMutex.Unlock()

	if _, ok := p.instances[id]; !ok {
		return fmt.Errorf("instance not found")
	}

	delete(p.instances, id)
	return nil
}
