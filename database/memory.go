package database

import (
	"fmt"
	"sync"

	"github.com/pborman/uuid"
)

// MemoryDatabase is a DB implementation that stores everything in memory,
// useful for testing.
type MemoryDatabase struct {
	mutex     sync.Mutex
	instances map[string]Instance
	tokens    []memoryToken
}

type memoryToken struct {
	Hash        []byte
	Salt        []byte
	Description string
}

func NewMemoryDatabase() *MemoryDatabase {
	return &MemoryDatabase{
		instances: make(map[string]Instance),
	}
}

func (db *MemoryDatabase) CreateInstance(instance Instance) (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	id := uuid.New()
	instance.ID = id
	db.instances[id] = instance

	return id, nil
}

func (db *MemoryDatabase) GetInstance(id string) (Instance, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	instance, ok := db.instances[id]

	if !ok {
		return Instance{}, ErrInstanceNotFound
	}

	return instance, nil
}

func (db *MemoryDatabase) UpdateInstance(instance Instance) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, ok := db.instances[instance.ID]
	if !ok {
		return ErrInstanceNotFound
	}

	db.instances[instance.ID] = instance

	return nil
}

func (db *MemoryDatabase) GetSaltAndHashForTokenID(tokenID uint64) ([]byte, []byte, error) {
	token := db.tokens[tokenID]

	return token.Salt, token.Hash, nil
}

func (db *MemoryDatabase) InsertToken(description string, hash, salt []byte) (uint64, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	id := uint64(len(db.tokens))

	db.tokens = append(db.tokens, memoryToken{
		Description: description,
		Hash:        hash,
		Salt:        salt,
	})

	return id, nil
}

func (db *MemoryDatabase) ListProviders() ([]Provider, error) {
	return nil, fmt.Errorf("provider listing not implemented for MemoryDatabase")
}
