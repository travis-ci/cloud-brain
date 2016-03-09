package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/database"
	"golang.org/x/crypto/scrypt"
)

func main() {
	app := cli.NewApp()
	app.Name = "cloudbrain-create-token"
	app.Usage = "Create a token for use with the Cloud Brain HTTP API"
	app.Action = mainAction
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "database-url",
			Usage:  "The URL for the PostgreSQL database to use",
			EnvVar: "CLOUDBRAIN_DATABASE_URL,DATABASE_URL",
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
	db := database.NewPostgresDB(pgdb)

	salt := make([]byte, 32)
	token := make([]byte, 16)
	_, err = rand.Read(salt)
	if err != nil {
		fmt.Printf("error: could not generate a random salt: %v\n", err)
		return
	}
	_, err = rand.Read(token)
	if err != nil {
		fmt.Printf("error: could not generate a random token: %v\n", err)
		return
	}

	hashed, err := scrypt.Key(token, salt, 16384, 8, 1, 32)
	if err != nil {
		fmt.Printf("error: could not scrypt: %v\n", err)
		return
	}

	tokenID, err := db.InsertToken(c.Args()[0], hashed, salt)
	if err != nil {
		fmt.Printf("error: couldn't insert the token into the database: %v\n", err)
		return
	}

	encodedToken := hex.EncodeToString(token)

	fmt.Printf("generated token: %d-%s\n", tokenID, encodedToken)
}
