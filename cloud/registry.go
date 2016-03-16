package cloud

import (
	"fmt"
	"sync"
)

var (
	backendRegistry      = map[string]*backendRegistryEntry{}
	backendRegistryMutex sync.Mutex
)

type backendRegistryEntry struct {
	Alias             string
	HumanReadableName string
	ProviderFunc      func([]byte) (Provider, error)
}

func registerProvider(alias, humanReadableName string, providerFunc func([]byte) (Provider, error)) {
	backendRegistryMutex.Lock()
	defer backendRegistryMutex.Unlock()

	backendRegistry[alias] = &backendRegistryEntry{
		Alias:             alias,
		HumanReadableName: humanReadableName,
		ProviderFunc:      providerFunc,
	}
}

// NewProvider creates a new provider given the alias and provider-specific
// configuration. The alias must match what is passed to registerProvider by the
// provider, and the configuration is passed to the provider for parsing.
func NewProvider(alias string, cfg []byte) (Provider, error) {
	backendRegistryMutex.Lock()
	defer backendRegistryMutex.Unlock()

	backend, ok := backendRegistry[alias]
	if !ok {
		return nil, fmt.Errorf("unknown cloud provider: %s", alias)
	}

	return backend.ProviderFunc(cfg)
}
