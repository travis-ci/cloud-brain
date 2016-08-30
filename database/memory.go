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

// NewMemoryDatabase creates and returns an empty MemoryDatabase.
func NewMemoryDatabase() *MemoryDatabase {
	return &MemoryDatabase{
		instances: make(map[string]Instance),
	}
}

// CreateInstance stores the instance in the database and returns the ID it
// generated for it. Never returns an error.
func (db *MemoryDatabase) CreateInstance(instance Instance) (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	id := uuid.New()
	instance.ID = id
	db.instances[id] = instance

	//TODO(emdantrim): log this action
	return id, nil
}

// RemoveInstance removes the instance from the database.
func (db *MemoryDatabase) RemoveInstance(instance Instance) (string, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	id := uuid.New()
	instance.ID = id
	delete(db.instances, id)

	//TODO(emdantrim): log this action
	return id, nil
}

// GetInstance returns the instance with the given ID, or ErrInstanceNotFound if
// no instance exists with that ID.
func (db *MemoryDatabase) GetInstance(id string) (Instance, error) {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	instance, ok := db.instances[id]

	if !ok {
		return Instance{}, ErrInstanceNotFound
	}

	//TODO(emdantrim): log this action
	return instance, nil
}

// UpdateInstance updates the instance with the given ID, or returns
// ErrInstanceNotFound if no instance with that ID exists.
func (db *MemoryDatabase) UpdateInstance(instance Instance) error {
	db.mutex.Lock()
	defer db.mutex.Unlock()

	_, ok := db.instances[instance.ID]
	if !ok {
		return ErrInstanceNotFound
		//TODO(emdantrim): log this action
	}

	db.instances[instance.ID] = instance

	//TODO(emdantrim): log this action
	return nil
}

// GetSaltAndHashForTokenID returns the salt and hash for a token with the given
// ID. Panics if the token doesn't exist.
func (db *MemoryDatabase) GetSaltAndHashForTokenID(tokenID uint64) ([]byte, []byte, error) {
	// TODO(henrikhodne): return an error if token doesn't exist
	token := db.tokens[tokenID]

	return token.Salt, token.Hash, nil
}

// InsertToken stores a token with the given description, hash and salt in the
// database. Never returns an error.
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

// ListProviders always returns an error. It's not implemented yet, it's just
// here to implement the database.DB interface.
func (db *MemoryDatabase) ListProviders() ([]Provider, error) {
	return nil, fmt.Errorf("provider listing not implemented for MemoryDatabase")
}

// CreateProvider always returns an error. It's not implemented yet, it's just
// here to implement the database.DB interface.
func (db *MemoryDatabase) CreateProvider(provider Provider) (string, error) {
	return "", fmt.Errorf("provider creation not implemented for MemoryDatabase")
}
