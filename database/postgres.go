package database

import (
	"crypto/rand"
	"database/sql"
	"fmt"

	"golang.org/x/crypto/nacl/secretbox"

	"github.com/pborman/uuid"
)

type PostgresDB struct {
	encryptionKey [32]byte
	db            *sql.DB
}

func NewPostgresDB(encryptionKey [32]byte, db *sql.DB) *PostgresDB {
	return &PostgresDB{
		encryptionKey: encryptionKey,
		db:            db,
	}
}

func (db *PostgresDB) CreateInstance(instance Instance) (string, error) {
	instance.ID = uuid.New()

	_, err := db.db.Exec(
		"INSERT INTO cloudbrain.instances (id, provider_name, image, state, ip_address, ssh_key) VALUES ($1, $2, $3, $4, $5, $6, $7)",
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
	)
	if err != nil {
		return "", err
	}

	return instance.ID, nil
}

func (db *PostgresDB) GetInstance(id string) (Instance, error) {
	instance := Instance{ID: id}
	var ipAddress, sshKey sql.NullString
	err := db.db.QueryRow(
		"SELECT provider_name, image, state, ip_address, ssh_key FROM cloudbrain.instances WHERE id = $1",
		id,
	).Scan(
		&instance.ProviderName,
		&instance.Image,
		&instance.State,
		&ipAddress,
		&sshKey,
	)
	if err == sql.ErrNoRows {
		return Instance{}, ErrInstanceNotFound
	}
	if err != nil {
		return Instance{}, err
	}

	instance.IPAddress = ipAddress.String
	instance.PublicSSHKey = sshKey.String

	return instance, nil
}

func (db *PostgresDB) UpdateInstance(instance Instance) error {
	_, err := db.db.Exec(
		"UPDATE cloudbrain.instances SET provider_name = $1, image = $2, state = $3, ip_address = $4, ssh_key = $5 WHERE id = $6",
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
		instance.ID,
	)
	return err
}

func (db *PostgresDB) GetSaltAndHashForTokenID(tokenID uint64) ([]byte, []byte, error) {
	var salt, hash []byte
	err := db.db.QueryRow(
		"SELECT token_salt, token_hash FROM cloudbrain.auth_tokens WHERE id = $1",
		tokenID,
	).Scan(&salt, &hash)

	return salt, hash, err
}

func (db *PostgresDB) InsertToken(description string, hash, salt []byte) (uint64, error) {
	var id uint64
	err := db.db.QueryRow(
		"INSERT INTO cloudbrain.auth_tokens (description, token_hash, token_salt) VALUES ($1, $2, $3) RETURNING id",
		description,
		hash,
		salt,
	).Scan(&id)
	if err != nil {
		return 0, err
	}

	return id, err
}

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

func (db *PostgresDB) decrypt(ciphertext []byte) ([]byte, bool) {
	if len(ciphertext) < 24 {
		return nil, false
	}

	var nonce [24]byte
	copy(nonce[:], ciphertext[0:24])
	box := ciphertext[24:]

	return secretbox.Open(nil, box, &nonce, &db.encryptionKey)
}

func (db *PostgresDB) encrypt(plaintext []byte) []byte {
	var nonce [24]byte
	_, err := rand.Read(nonce[:])
	if err != nil {
		return nil
	}

	out := secretbox.Seal(nil, plaintext, &nonce, &db.encryptionKey)

	return append(nonce[:], out...)
}
