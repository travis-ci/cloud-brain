package database

import (
	"sync"

	"github.com/pborman/uuid"
)

// MemoryDatabase is a DB implementation that stores everything in memory,
// useful for testing.
type MemoryDatabase struct {
	instancesMutex sync.Mutex
	instances      map[string]Instance
}

func NewMemoryDatabase() *MemoryDatabase {
	return &MemoryDatabase{
		instances: make(map[string]Instance),
	}
}

func (db *MemoryDatabase) CreateInstance(instance Instance) (string, error) {
	db.instancesMutex.Lock()
	defer db.instancesMutex.Unlock()

	id := uuid.New()
	instance.ID = id
	db.instances[id] = instance

	return id, nil
}

func (db *MemoryDatabase) GetInstance(id string) (Instance, error) {
	db.instancesMutex.Lock()
	defer db.instancesMutex.Unlock()

	instance, ok := db.instances[id]

	if !ok {
		return Instance{}, ErrInstanceNotFound
	}

	return instance, nil
}

func (db *MemoryDatabase) GetInstanceByProviderID(providerName, providerID string) (Instance, error) {
	db.instancesMutex.Lock()
	defer db.instancesMutex.Unlock()

	for _, instance := range db.instances {
		if instance.ProviderID == providerID && instance.Provider == providerName {
			return instance, nil
		}
	}

	return Instance{}, ErrInstanceNotFound
}

func (db *MemoryDatabase) UpdateInstance(instance Instance) error {
	db.instancesMutex.Lock()
	defer db.instancesMutex.Unlock()

	_, ok := db.instances[instance.ID]
	if !ok {
		return ErrInstanceNotFound
	}

	db.instances[instance.ID] = instance

	return nil
}
