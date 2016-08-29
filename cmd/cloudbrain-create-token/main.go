package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"

	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"golang.org/x/crypto/scrypt"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:      "cloudbrain-create-token",
		Version:   cloudbrain.VersionString,
		Copyright: cloudbrain.CopyrightString,
		Usage:     "Create a token for use with the Cloud Brain HTTP API",
		Action:    mainAction,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "database-url",
				Usage:   "The URL for the PostgreSQL database to use",
				EnvVars: []string{"CLOUDBRAIN_DATABASE_URL", "DATABASE_URL"},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
}

func mainAction(c *cli.Context) error {
	if c.String("database-url") == "" {
		return fmt.Errorf("error: the DATABASE_URL environment variable must be set\n")
	}
	pgdb, err := sql.Open("postgres", c.String("database-url"))
	if err != nil {
		return fmt.Errorf("error: could not connect to the database: %v\n", err)
	}
	db := database.NewPostgresDB([32]byte{}, pgdb)

	salt := make([]byte, 32)
	token := make([]byte, 16)
	_, err = rand.Read(salt)
	if err != nil {
		return fmt.Errorf("error: could not generate a random salt: %v\n", err)
	}
	_, err = rand.Read(token)
	if err != nil {
		return fmt.Errorf("error: could not generate a random token: %v\n", err)
	}

	hashed, err := scrypt.Key(token, salt, 16384, 8, 1, 32)
	if err != nil {
		return fmt.Errorf("error: could not scrypt: %v\n", err)
	}

	tokenID, err := db.InsertToken(c.Args().Get(0), hashed, salt)
	if err != nil {
		return fmt.Errorf("error: couldn't insert the token into the database: %v\n", err)
	}

	encodedToken := hex.EncodeToString(token)

	fmt.Printf("generated token: %d-%s\n", tokenID, encodedToken)

	return nil
}
