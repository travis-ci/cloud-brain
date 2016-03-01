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
		"INSERT INTO cloudbrain.instances (id, provider, provider_id, image, state, ip_address) VALUES ($1, $2, $3, $4, $5, $6)",
		instance.ID,
		instance.Provider,
		instance.ProviderID,
		instance.Image,
		instance.State,
		sql.NullString{
			String: instance.IPAddress,
			Valid:  instance.IPAddress != "",
		},
	)
	if err != nil {
		return "", err
	}

	return instance.ID, nil
}

func (db *PostgresDB) GetInstance(id string) (Instance, error) {
	instance := Instance{ID: id}
	var ipAddress sql.NullString
	err := db.db.QueryRow(
		"SELECT provider, provider_id, image, state, ip_address FROM cloudbrain.instances WHERE id = $1",
		id,
	).Scan(
		&instance.Provider,
		&instance.ProviderID,
		&instance.Image,
		&instance.State,
		&ipAddress,
	)
	if err != nil {
		return Instance{}, err
	}

	instance.IPAddress = ipAddress.String

	return instance, nil
}

func (db *PostgresDB) GetInstanceByProviderID(providerName, providerID string) (Instance, error) {
	instance := Instance{
		Provider:   providerName,
		ProviderID: providerID,
	}
	var ipAddress sql.NullString
	err := db.db.QueryRow(
		"SELECT id, image, state, ip_address FROM cloudbrain.instances WHERE provider = $1 AND provider_id = $2",
		providerName,
		providerID,
	).Scan(
		&instance.ID,
		&instance.Image,
		&instance.State,
		&ipAddress,
	)
	if err != nil {
		return Instance{}, err
	}

	instance.IPAddress = ipAddress.String

	return instance, nil
}

func (db *PostgresDB) UpdateInstance(instance Instance) error {
	_, err := db.db.Exec(
		"UPDATE cloudbrain.instances SET provider = $1, provider_id = $2, image = $3, state = $4, ip_address = $5 WHERE id = $6",
		instance.Provider,
		instance.ProviderID,
		instance.Image,
		instance.State,
		sql.NullString{
			String: instance.IPAddress,
			Valid:  instance.IPAddress != "",
		},
		instance.ID,
	)
	return err
}
