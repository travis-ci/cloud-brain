package cloud

import (
	"fmt"
	"sync"
)

var (
	backendRegistry      = map[string]*Backend{}
	backendRegistryMutex sync.Mutex
)

type Backend struct {
	Alias             string
	HumanReadableName string
	ProviderFunc      func([]byte) (Provider, error)
}

func Register(alias, humanReadableName string, providerFunc func([]byte) (Provider, error)) {
	backendRegistryMutex.Lock()
	defer backendRegistryMutex.Unlock()

	backendRegistry[alias] = &Backend{
		Alias:             alias,
		HumanReadableName: humanReadableName,
		ProviderFunc:      providerFunc,
	}
}

func NewProvider(alias string, cfg []byte) (Provider, error) {
	backendRegistryMutex.Lock()
	defer backendRegistryMutex.Unlock()

	backend, ok := backendRegistry[alias]
	if !ok {
		return nil, fmt.Errorf("unknown cloud provider: %s", alias)
	}

	return backend.ProviderFunc(cfg)
}
