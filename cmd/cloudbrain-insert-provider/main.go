package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:      "cloudbrain-insert-provider",
		Version:   cloudbrain.VersionString,
		Copyright: cloudbrain.CopyrightString,
		Usage:     "Insert configuration for a provider into the database",
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
			&cli.StringFlag{
				Name:    "gce-account-json",
				Usage:   "A path pointing to the GCE account JSON file",
				EnvVars: []string{"CLOUDBRAIN_GCE_ACCOUNT_JSON"},
			},
			&cli.StringFlag{
				Name:    "gce-project-id",
				Usage:   "The GCE project ID for the project to boot instances in",
				EnvVars: []string{"CLOUDBRAIN_GCE_PROJECT_ID"},
			},
			&cli.StringFlag{
				Name:    "gce-image-project-id",
				Usage:   "The GCE project ID for the project containing the build environment images",
				EnvVars: []string{"CLOUDBRAIN_GCE_IMAGE_PROJECT_ID"},
			},
			&cli.StringFlag{
				Name:    "gce-zone",
				Usage:   "The GCE zone to boot instances in",
				Value:   "us-central1-a",
				EnvVars: []string{"CLOUDBRAIN_GCE_ZONE"},
			},
			&cli.StringFlag{
				Name:    "gce-standard-machine-type",
				Usage:   "The machine type to use for 'standard' instances",
				Value:   "n1-standard-2",
				EnvVars: []string{"CLOUDBRAIN_GCE_STANDARD_MACHINE_TYPE"},
			},
			&cli.StringFlag{
				Name:    "gce-premium-machine-type",
				Usage:   "The machine type to use for 'premium' instances",
				Value:   "n1-standard-4",
				EnvVars: []string{"CLOUDBRAIN_GCE_PREMIUM_MACHINE_TYPE"},
			},
			&cli.StringFlag{
				Name:    "gce-network",
				Usage:   "The GCE network to connect instances to",
				Value:   "default",
				EnvVars: []string{"CLOUDBRAIN_GCE_NETWORK"},
			},
			&cli.IntFlag{
				Name:    "gce-disk-size",
				Usage:   "The GCE disk size in GiB",
				Value:   30,
				EnvVars: []string{"CLOUDBRAIN_GCE_DISK_SIZE"},
			},
			&cli.BoolFlag{
				Name:    "gce-auto-implode",
				Usage:   "Enable to make the instance power off after gce-auto-implode-time if it's still running",
				EnvVars: []string{"CLOUDBRAIN_GCE_AUTO_IMPLODE"},
			},
			&cli.DurationFlag{
				Name:    "gce-auto-implode-time",
				Usage:   "How long to wait before auto-imploding. Will be rounded down to the nearest minute.",
				EnvVars: []string{"CLOUDBRAIN_GCE_AUTO_IMPLODE_TIME"},
			},
			&cli.BoolFlag{
				Name:    "gce-preemptible",
				Usage:   "Enable to use GCE preemptible instances",
				EnvVars: []string{"CLOUDBRAIN_GCE_PREEMPTIBLE"},
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

	accountJSON, err := loadGoogleAccountJSON(c.String("gce-account-json"))
	if err != nil {
		return fmt.Errorf("error: couldn't load GCE account JSON file: %v", err)
	}

	providerConfig := cloud.GCEProviderConfiguration{
		AccountJSON:         accountJSON,
		ProjectID:           c.String("gce-project-id"),
		ImageProjectID:      c.String("gce-image-project-id"),
		Zone:                c.String("gce-zone"),
		StandardMachineType: c.String("gce-standard-machine-type"),
		PremiumMachineType:  c.String("gce-premium-machine-type"),
		Network:             c.String("gce-network"),
		DiskSize:            int64(c.Int("gce-disk-size")),
		AutoImplode:         c.Bool("gce-auto-implode"),
		AutoImplodeTime:     c.Duration("gce-auto-implode-time"),
		Preemptible:         c.Bool("gce-preemptible"),
	}

	jsonConfig, err := json.Marshal(providerConfig)
	if err != nil {
		return fmt.Errorf("error: couldn't JSON-encode provider configuration: %v", err)
	}

	providerName := c.String("provider-name")
	if providerName == "" {
		return fmt.Errorf("error: provider name can't be blank")
	}

	id, err := db.CreateProvider(database.Provider{
		Type:   "gce",
		Name:   providerName,
		Config: jsonConfig,
	})

	if err != nil {
		return fmt.Errorf("error: couldn't insert provider in database: %v", err)
	}

	fmt.Printf("created provider %s with ID %s\n", providerName, id)
	return nil
}

func loadGoogleAccountJSON(filename string) (cloud.GCEAccountJSON, error) {
	file, err := os.Open(filename)
	if err != nil {
		return cloud.GCEAccountJSON{}, err
	}
	defer file.Close()

	var accountJSON cloud.GCEAccountJSON
	err = json.NewDecoder(file).Decode(&accountJSON)
	return accountJSON, err
}
