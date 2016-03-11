package main

import (
	"database/sql"
	"encoding/hex"
	"os"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/garyburd/redigo/redis"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/worker"
)

func main() {
	app := cli.NewApp()
	app.Name = "cloudbrain-http"
	app.Usage = "Run the HTTP server part of Cloud Brain"
	app.Action = mainAction
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "redis-url",
			EnvVar: "CLOUDBRAIN_REDIS_URL,REDIS_URL",
		},
		cli.IntFlag{
			Name:   "redis-max-idle",
			Value:  3,
			Usage:  "The maximum number of idle Redis connections",
			EnvVar: "CLOUDBRAIN_REDIS_MAX_IDLE",
		},
		cli.IntFlag{
			Name:   "redis-max-active",
			Value:  5,
			Usage:  "The maximum number of active Redis connections",
			EnvVar: "CLOUDBRAIN_REDIS_MAX_ACTIVE",
		},
		cli.DurationFlag{
			Name:   "redis-idle-timeout",
			Value:  3 * time.Minute,
			EnvVar: "CLOUDBRAIN_REDIS_IDLE_TIMEOUT",
		},
		cli.StringFlag{
			Name:   "redis-worker-prefix",
			Value:  "cloud-brain:worker",
			Usage:  "The Redis key prefix to use for keys used by the background workers",
			EnvVar: "CLOUDBRAIN_REDIS_WORKER_PREFIX",
		},
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
	}

	app.Run(os.Args)
}

func mainAction(c *cli.Context) {
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
	workerBackend := worker.NewRedisWorker(redisPool, c.String("redis-worker-prefix"))

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

	core, err := cloudbrain.NewCore(&cloudbrain.CoreConfig{
		DB:            db,
		WorkerBackend: workerBackend,
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("couldn't configure core")
	}

	err = worker.Run(ctx, "create", workerBackend, worker.WorkerFunc(core.ProviderCreateInstance))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("create worker crashed")
	}
}
