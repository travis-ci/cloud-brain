// Package database implements a database to store instance information in
package database

import "errors"

var ErrInstanceNotFound = errors.New("instance not found")

type DB interface {
	// Inserts the instance into the database, returns the id or an error.
	CreateInstance(instance Instance) (string, error)

	// Retrieves the instance by its ID, or returns an error
	GetInstance(id string) (Instance, error)

	// Retrieves the instance by its provider name and provider ID
	GetInstanceByProviderID(provider, providerID string) (Instance, error)

	// Updates the instance with the given ID
	UpdateInstance(instance Instance) error
}

type Instance struct {
	ID         string
	Provider   string
	ProviderID string
	Image      string
	State      string
	IPAddress  string
}
