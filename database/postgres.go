package database

import (
	"database/sql"

	"github.com/pborman/uuid"
)

type PostgresDB struct {
	db *sql.DB
}

func NewPostgresDB(db *sql.DB) *PostgresDB {
	return &PostgresDB{
		db: db,
	}
}

func (db *PostgresDB) CreateInstance(instance Instance) (string, error) {
	instance.ID = uuid.New()

	_, err := db.db.Exec(
		"INSERT INTO cloudbrain.instances (id, provider, provider_id, image, state, ip_address, ssh_key) VALUES ($1, $2, $3, $4, $5, $6, $7)",
		instance.ID,
		instance.Provider,
		instance.ProviderID,
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
		"SELECT provider, provider_id, image, state, ip_address, ssh_key FROM cloudbrain.instances WHERE id = $1",
		id,
	).Scan(
		&instance.Provider,
		&instance.ProviderID,
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

func (db *PostgresDB) GetInstanceByProviderID(providerName, providerID string) (Instance, error) {
	instance := Instance{
		Provider:   providerName,
		ProviderID: providerID,
	}
	var ipAddress, sshKey sql.NullString
	err := db.db.QueryRow(
		"SELECT id, image, state, ip_address, ssh_key FROM cloudbrain.instances WHERE provider = $1 AND provider_id = $2",
		providerName,
		providerID,
	).Scan(
		&instance.ID,
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
		"UPDATE cloudbrain.instances SET provider = $1, provider_id = $2, image = $3, state = $4, ip_address = $5, ssh_key = $6 WHERE id = $7",
		instance.Provider,
		instance.ProviderID,
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
