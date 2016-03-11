package main

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/database"
)

func main() {
	app := cli.NewApp()
	app.Name = "cloudbrain-insert-provider"
	app.Usage = "Insert configuration for a provider into the database"
	app.Action = mainAction
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "database-url",
			Usage:  "The URL for the PostgreSQL database to use",
			EnvVar: "CLOUDBRAIN_DATABASE_URL,DATABASE_URL",
		},
		cli.StringFlag{
			Name:   "database-encryption-key",
			Usage:  "The database encryption key, hex-encoded",
			EnvVar: "CLOUDBRAIN_DATABASE_ENCRYPTION_KEY",
		},
		cli.StringFlag{
			Name:   "gce-account-json",
			Usage:  "A path pointing to the GCE account JSON file",
			EnvVar: "CLOUDBRAIN_GCE_ACCOUNT_JSON",
		},
		cli.StringFlag{
			Name:   "gce-project-id",
			Usage:  "The GCE project ID for the project to boot instances in",
			EnvVar: "CLOUDBRAIN_GCE_PROJECT_ID",
		},
		cli.StringFlag{
			Name:   "gce-image-project-id",
			Usage:  "The GCE project ID for the project containing the build environment images",
			EnvVar: "CLOUDBRAIN_GCE_IMAGE_PROJECT_ID",
		},
		cli.StringFlag{
			Name:   "gce-zone",
			Usage:  "The GCE zone to boot instances in",
			Value:  "us-central1-a",
			EnvVar: "CLOUDBRAIN_GCE_ZONE",
		},
		cli.StringFlag{
			Name:   "gce-standard-machine-type",
			Usage:  "The machine type to use for 'standard' instances",
			Value:  "n1-standard-2",
			EnvVar: "CLOUDBRAIN_GCE_STANDARD_MACHINE_TYPE",
		},
		cli.StringFlag{
			Name:   "gce-premium-machine-type",
			Usage:  "The machine type to use for 'premium' instances",
			Value:  "n1-standard-4",
			EnvVar: "CLOUDBRAIN_GCE_PREMIUM_MACHINE_TYPE",
		},
		cli.StringFlag{
			Name:   "gce-network",
			Usage:  "The GCE network to connect instances to",
			Value:  "default",
			EnvVar: "CLOUDBRAIN_GCE_NETWORK",
		},
		cli.IntFlag{
			Name:   "gce-disk-size",
			Usage:  "The GCE disk size in GiB",
			Value:  30,
			EnvVar: "CLOUDBRAIN_GCE_DISK_SIZE",
		},
		cli.BoolFlag{
			Name:   "gce-auto-implode",
			Usage:  "Enable to make the instance power off after gce-auto-implode-time if it's still running",
			EnvVar: "CLOUDBRAIN_GCE_AUTO_IMPLODE",
		},
		cli.DurationFlag{
			Name:   "gce-auto-implode-time",
			Usage:  "How long to wait before auto-imploding. Will be rounded down to the nearest minute.",
			EnvVar: "CLOUDBRAIN_GCE_AUTO_IMPLODE_TIME",
		},
		cli.BoolFlag{
			Name:   "gce-preemptible",
			Usage:  "Enable to use GCE preemptible instances",
			EnvVar: "CLOUDBRAIN_GCE_PREEMPTIBLE",
		},
	}

	app.Run(os.Args)
}

func mainAction(c *cli.Context) {
	if c.String("database-url") == "" {
		fmt.Printf("error: the DATABASE_URL environment variable must be set\n")
		return
	}
	pgdb, err := sql.Open("postgres", c.String("database-url"))
	if err != nil {
		fmt.Printf("error: could not connect to the database: %v\n", err)
		return
	}

	var encryptionKey [32]byte
	keySlice, err := hex.DecodeString(c.String("database-encryption-key"))
	if err != nil {
		fmt.Printf("error: couldn't decode the database encryption key: %v\n", err)
		return
	}
	copy(encryptionKey[:], keySlice[0:32])

	db := database.NewPostgresDB(encryptionKey, pgdb)

	accountJSON, err := loadGoogleAccountJSON(c.String("gce-account-json"))
	if err != nil {
		fmt.Printf("error: couldn't load GCE account JSON file: %v\n", err)
		return
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
		fmt.Printf("error: couldn't JSON-encode provider configuration: %v\n", err)
		return
	}

	id, err := db.CreateProvider(database.Provider{
		Type:   "gce",
		Config: jsonConfig,
	})

	if err != nil {
		fmt.Printf("error: couldn't insert provider in database: %v\n", err)
		return
	}

	fmt.Printf("created provider with ID %s\n", id)
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
