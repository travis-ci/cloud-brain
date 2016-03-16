// Package database implements a database to store instance information in
package database

import "errors"

// ErrInstanceNotFound is returned from DB methods when an instance with the
// given ID could not be found.
var ErrInstanceNotFound = errors.New("instance not found")

// DB is implemented by the supported database backends.
type DB interface {
	// Inserts the instance into the database, returns the id or an error.
	CreateInstance(instance Instance) (string, error)

	// Retrieves the instance by its ID, or returns an error
	GetInstance(id string) (Instance, error)

	// Updates the instance with the given ID
	UpdateInstance(instance Instance) error

	// GetHashedToken gets the salt and the hashed token for a given token ID.
	// The returned attributes are salt, hash and an error.
	GetSaltAndHashForTokenID(tokenID uint64) ([]byte, []byte, error)

	// Insert a token into the database, returns the ID of the token
	InsertToken(description string, hash, salt []byte) (uint64, error)

	// List all the providers in the database
	ListProviders() ([]Provider, error)

	// Inserts the provider into the database, returns the id or an error. The
	// id will be automatically generated if one is not supplied.
	CreateProvider(provider Provider) (string, error)
}

// Instance contains the data stored about a compute instance in the database.
type Instance struct {
	ID           string
	ProviderName string
	Image        string
	InstanceType string
	PublicSSHKey string
	State        string
	IPAddress    string
}

// Provider contains the data stored about a cloud provider in the database.
type Provider struct {
	// ID is a UUID for this provider
	ID string

	// Type should match up with the alias passed into cloud.NewProvider
	Type string

	// Name is a unique name passed to the HTTP API to specify which provider to
	// create an instance on.
	Name string

	// Config is a provider-specific configuration, passed to cloud.NewProvider.
	Config []byte
}
