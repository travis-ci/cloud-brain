package database

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/pborman/uuid"
)

// PostgresDB is a DB implementation backed by a Postgres database.
type PostgresDB struct {
	encryptionKey [32]byte
	db            *sql.DB
}

// NewPostgresDB creates a new PostgresDB that uses the given sql.DB (must be a
// postgres connection). The encryptionKey must be provided if getting encrypted
// data, but can be nil if no encrypted data is needed.
func NewPostgresDB(encryptionKey [32]byte, db *sql.DB) *PostgresDB {
	return &PostgresDB{
		encryptionKey: encryptionKey,
		db:            db,
	}
}

// CreateInstance stores the given instance in teh database. A new UUID is
// generated for it and returned. If an error occurrs, the empty string and the
// error is returned.
func (db *PostgresDB) CreateInstance(instance Instance) (string, error) {
	instance.ID = uuid.New()

	_, err := db.db.Exec(
		"INSERT INTO cloudbrain.instances (id, provider_name, image, state, ip_address, ssh_key, upstream_id, error_reason) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
		instance.ID,
		instance.ProviderName,
		instance.Image,
		instance.State,
		sql.NullString{
			String: instance.IPAddress,
			Valid:  instance.IPAddress != "",
		},
		sql.NullString{
			String: instance.PublicSSHKey,
			Valid:  instance.PublicSSHKey != "",
		},
		sql.NullString{
			String: instance.UpstreamID,
			Valid:  instance.UpstreamID != "",
		},
		sql.NullString{
			String: instance.ErrorReason,
			Valid:  instance.ErrorReason != "",
		},
	)
	if err != nil {
		return "", err
	}

	return instance.ID, nil
}

// RemoveInstance deletes the given instance from the database. If an
// error occurs, the empty string and the error is returned.
func (db *PostgresDB) RemoveInstance(instance Instance) (string, error) {

	_, err := db.db.Exec(
		"DELETE FROM cloudbrain.instances WHERE id = $1",
		instance.ID,
	)
	if err != nil {
		return "", err
	}

	return instance.ID, nil
}

// GetInstance returns the instance with the given ID from the database. If no
// instance with the given ID exists, ErrInstanceNotFound is returned. If an
// error occurs, then an empty Instance struct and the error is returned.
func (db *PostgresDB) GetInstance(id string) (Instance, error) {
	instance := Instance{ID: id}
	var ipAddress, sshKey, upstreamID, errorReason sql.NullString
	err := db.db.QueryRow(
		"SELECT provider_name, image, state, ip_address, ssh_key, upstream_id, error_reason FROM cloudbrain.instances WHERE id = $1",
		id,
	).Scan(
		&instance.ProviderName,
		&instance.Image,
		&instance.State,
		&ipAddress,
		&sshKey,
		&upstreamID,
		&errorReason,
	)
	if err == sql.ErrNoRows {
		return Instance{}, ErrInstanceNotFound
	}
	if err != nil {
		return Instance{}, err
	}

	instance.IPAddress = ipAddress.String
	instance.PublicSSHKey = sshKey.String
	instance.UpstreamID = upstreamID.String
	instance.ErrorReason = errorReason.String

	return instance, nil
}

// GetInstancesByState returns a slice of instances for a given state
func (db *PostgresDB) GetInstancesByState(state string) ([]Instance, error) {
	var instances []Instance

	rows, err := db.db.Query("SELECT id, provider_name, image, state, ip_address, ssh_key, upstream_id, error_reason FROM cloudbrain.instances WHERE state = $1", state)
	if err != nil {
		return instances, err
	}
	for rows.Next() {
		instance := &Instance{}
		var ipAddress, sshKey, upstreamID, errorReason sql.NullString
		err := rows.Scan(
			&instance.ID,
			&instance.ProviderName,
			&instance.Image,
			&instance.State,
			&ipAddress,
			&sshKey,
			&upstreamID,
			&errorReason,
		)
		if err == sql.ErrNoRows {
			return instances, ErrInstanceNotFound
		}
		if err != nil {
			return instances, err
		}

		instance.IPAddress = ipAddress.String
		instance.PublicSSHKey = sshKey.String
		instance.UpstreamID = upstreamID.String
		instance.ErrorReason = errorReason.String

		instances = append(instances, *instance)
	}
	if err := rows.Err(); err != nil {
		return instances, err
	}

	return instances, nil
}

// UpdateInstance updates the instane with the given ID in the database to match
// the given attributes. Returns ErrInstanceNotFound if an instance with the
// given ID isn't found.
//
// BUG(henrikhodne): ErrInstanceNotFound is not returned when an instance with
// the given ID doesn't exist.
func (db *PostgresDB) UpdateInstance(instance Instance) error {
	_, err := db.db.Exec(
		"UPDATE cloudbrain.instances SET provider_name = $1, image = $2, state = $3, ip_address = $4, ssh_key = $5, upstream_id = $6, error_reason = $7 WHERE id = $8",
		instance.ProviderName,
		instance.Image,
		instance.State,
		sql.NullString{
			String: instance.IPAddress,
			Valid:  instance.IPAddress != "",
		},
		sql.NullString{
			String: instance.PublicSSHKey,
			Valid:  instance.PublicSSHKey != "",
		},
		sql.NullString{
			String: instance.UpstreamID,
			Valid:  instance.UpstreamID != "",
		},
		sql.NullString{
			String: instance.ErrorReason,
			Valid:  instance.ErrorReason != "",
		},
		instance.ID,
	)
	return err
}

// GetSaltAndHashForTokenID returns the salt and hash for the token with the
// given ID.
//
// BUG(henrikhodne): Should return a special error if no token with the given
// ID exists.
func (db *PostgresDB) GetSaltAndHashForTokenID(tokenID uint64) (salt []byte, hash []byte, err error) {
	err = db.db.QueryRow(
		"SELECT token_salt, token_hash FROM cloudbrain.auth_tokens WHERE id = $1",
		tokenID,
	).Scan(&salt, &hash)

	return salt, hash, err
}

// InsertToken inserts a token into the database with the given description, hash
// and salt.
func (db *PostgresDB) InsertToken(description string, hash, salt []byte) (uint64, error) {
	var id uint64
	err := db.db.QueryRow(
		"INSERT INTO cloudbrain.auth_tokens (description, token_hash, token_salt) VALUES ($1, $2, $3) RETURNING id",
		description,
		hash,
		salt,
	).Scan(&id)
	// TODO: how bout some dates?
	if err != nil {
		return 0, err
	}

	return id, err
}

// ListProviders returns a list of all the providers and their configurations.
//
// A valid encryption key must have been provided to NewPostgresDB for this to
// work, or an error will always be returned.
func (db *PostgresDB) ListProviders() ([]Provider, error) {
	rows, err := db.db.Query("SELECT id, type, name, config FROM cloudbrain.providers")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var providers []Provider
	for rows.Next() {
		var provider Provider
		var encryptedConfig []byte
		err := rows.Scan(&provider.ID, &provider.Type, &provider.Name, &encryptedConfig)
		if err != nil {
			return nil, err
		}

		var ok bool
		provider.Config, ok = db.decrypt(encryptedConfig)
		if !ok {
			return nil, fmt.Errorf("unable to decrypt config for provider %s", provider.ID)
		}

		providers = append(providers, provider)
	}

	if rows.Err() != nil {
		return nil, rows.Err()
	}

	return providers, nil
}

// CreateProvider inserts a provider with the given data into the database.
//
// A valid encryption key must have been provided to NewPostgresDB for this to
// work, or the stored configuration will not be valid.
func (db *PostgresDB) CreateProvider(provider Provider) (string, error) {
	if provider.ID == "" {
		provider.ID = uuid.New()
	}

	encryptedConfig := db.encrypt(provider.Config)

	_, err := db.db.Exec(
		"INSERT INTO cloudbrain.providers (id, type, name, config) VALUES ($1, $2, $3, $4)",
		provider.ID,
		provider.Type,
		provider.Name,
		encryptedConfig,
	)
	if err != nil {
		return "", err
	}

	return provider.ID, nil
}

// GetProviderByName fetches a provider and decrypts the config
func (db *PostgresDB) GetProviderByName(id string) (*Provider, error) {
	provider := &Provider{}
	var config []byte

	err := db.db.QueryRow(
		"SELECT id, name, type, config FROM cloudbrain.providers WHERE name = $1",
		id,
	).Scan(
		&provider.ID,
		&provider.Name,
		&provider.Type,
		&config,
	)
	if err != nil {
		return nil, err
	}

	config, valid := db.decrypt(config)
	if !valid {
		return nil, errors.New("could not decrypt provider config")
	}

	provider.Config = config

	return provider, nil
}

// decrypt is used to decrypt encrypted data using the encryption key
//
// Uses NaCl's secretbox algorithm. The ciphertext must start with the 24-byte
// nonce.
//
// Returns (nil, false) if the ciphertext or the key is invalid.
func (db *PostgresDB) decrypt(ciphertext []byte) ([]byte, bool) {
	if len(ciphertext) < 24 {
		return nil, false
	}

	var nonce [24]byte
	copy(nonce[:], ciphertext[0:24])
	box := ciphertext[24:]

	return secretbox.Open(nil, box, &nonce, &db.encryptionKey)
}

// encrypt is used to encrypt data, which can be decrypted again by decrypt if
// the same encryption key is used.
func (db *PostgresDB) encrypt(plaintext []byte) []byte {
	var nonce [24]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil
	}

	out := secretbox.Seal(nil, plaintext, &nonce, &db.encryptionKey)

	return append(nonce[:], out...)
}
