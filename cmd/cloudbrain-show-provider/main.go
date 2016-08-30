package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:      "cloudbrain-show-provider",
		Version:   cloudbrain.VersionString,
		Copyright: cloudbrain.CopyrightString,
		Usage:     "Show configuration for a provider",
		Action:    mainAction,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "database-url",
				Usage:   "The URL for the PostgreSQL database to use",
				EnvVars: []string{"CLOUDBRAIN_DATABASE_URL", "DATABASE_URL"},
			},
			&cli.StringFlag{
				Name:    "database-encryption-key",
				Usage:   "The database encryption key, hex-encoded",
				EnvVars: []string{"CLOUDBRAIN_DATABASE_ENCRYPTION_KEY"},
			},
			&cli.StringFlag{
				Name:    "provider-name",
				Usage:   "The name to assign to the provider being added",
				EnvVars: []string{"CLOUDBRAIN_PROVIDER_NAME"},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}

func mainAction(c *cli.Context) error {
	if c.String("database-url") == "" {
		return fmt.Errorf("error: the DATABASE_URL environment variable must be set")
	}
	pgdb, err := sql.Open("postgres", c.String("database-url"))
	if err != nil {
		return fmt.Errorf("error: could not connect to the database: %v", err)
	}

	var encryptionKey [32]byte
	keySlice, err := hex.DecodeString(c.String("database-encryption-key"))
	if err != nil {
		return fmt.Errorf("error: couldn't decode the database encryption key: %v", err)
	}
	copy(encryptionKey[:], keySlice[0:32])

	db := database.NewPostgresDB(encryptionKey, pgdb)

	providerName := c.String("provider-name")
	if providerName == "" {
		return fmt.Errorf("error: provider name can't be blank")
	}

	provider, err := db.GetProviderByName(providerName)
	if err != nil {
		return fmt.Errorf("error: couldn't load provider: %v", err)
	}

	fmt.Printf("ID: %v\n", provider.ID)
	fmt.Printf("Type: %v\n", provider.Type)
	fmt.Printf("Name: %v\n", provider.Name)
	fmt.Printf("Config:\n%s\n", provider.Config)

	return nil
}
