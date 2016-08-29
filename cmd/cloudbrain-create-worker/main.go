package main

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/background"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{}
	app.Name = "cloudbrain-create-worker"
	app.Version = cloudbrain.VersionString
	app.Copyright = cloudbrain.CopyrightString
	app.Usage = "Run the 'create instance' background worker"
	app.Action = mainAction
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    "redis-url",
			EnvVars: []string{"CLOUDBRAIN_REDIS_URL", "REDIS_URL"},
		},
		&cli.IntFlag{
			Name:    "redis-max-idle",
			Value:   3,
			Usage:   "The maximum number of idle Redis connections",
			EnvVars: []string{"CLOUDBRAIN_REDIS_MAX_IDLE"},
		},
		&cli.IntFlag{
			Name:    "redis-max-active",
			Value:   5,
			Usage:   "The maximum number of active Redis connections",
			EnvVars: []string{"CLOUDBRAIN_REDIS_MAX_ACTIVE"},
		},
		&cli.DurationFlag{
			Name:    "redis-idle-timeout",
			Value:   3 * time.Minute,
			EnvVars: []string{"CLOUDBRAIN_REDIS_IDLE_TIMEOUT"},
		},
		&cli.StringFlag{
			Name:    "redis-worker-prefix",
			Value:   "cloud-brain:worker",
			Usage:   "The Redis key prefix to use for keys used by the background workers",
			EnvVars: []string{"CLOUDBRAIN_REDIS_WORKER_PREFIX"},
		},
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
	}

	err := app.Run(os.Args)
	if err != nil {
		fmt.Printf("%v", err)
		os.Exit(1)
	}
}

func mainAction(c *cli.Context) error {
	ctx := context.Background()
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	if c.String("redis-url") == "" {
		cbcontext.LoggerFromContext(ctx).Fatal("redis-url flag is required")
	}
	redisURL := c.String("redis-url")
	redisPool := &redis.Pool{
		MaxIdle:     c.Int("redis-max-idle"),
		MaxActive:   c.Int("redis-max-active"),
		IdleTimeout: c.Duration("redis-idle-timeout"),
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(redisURL)
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	backgroundBackend := background.NewRedisBackend(redisPool, c.String("redis-worker-prefix"))

	if c.String("database-url") == "" {
		cbcontext.LoggerFromContext(ctx).Fatal("database-url flag is required")
	}
	pgdb, err := sql.Open("postgres", c.String("database-url"))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("couldn't connect to postgres")
	}

	var encryptionKey [32]byte
	keySlice, err := hex.DecodeString(c.String("database-encryption-key"))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("couldn't decode database encryption key")
	}
	copy(encryptionKey[:], keySlice[0:32])

	db := database.NewPostgresDB(encryptionKey, pgdb)

	core := cloudbrain.NewCore(db, backgroundBackend)

	err = background.Run(ctx, "create", backgroundBackend, background.WorkerFunc(core.ProviderCreateInstance))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("create worker crashed")
	}
	return nil
}
