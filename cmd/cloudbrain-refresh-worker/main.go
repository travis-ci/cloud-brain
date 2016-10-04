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
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:      "cloudbrain-refresh-worker",
		Version:   cloudbrain.VersionString,
		Copyright: cloudbrain.CopyrightString,
		Usage:     "Run the 'refresh providers' background worker",
		Action:    mainAction,
		Flags: []cli.Flag{
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
			&cli.DurationFlag{
				Name:    "refresh-interval",
				Usage:   "The interval at which to refresh the cached instances",
				Value:   5 * time.Second,
				EnvVars: []string{"CLOUDBRAIN_REFRESH_INTERVAL"},
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

	redisWorkerPrefix := c.String("redis-worker-prefix")
	core := cloudbrain.NewCore(db, redisPool, redisWorkerPrefix)

	var errorCount uint
	for {
		err := core.ProviderRefresh(ctx)
		if err != nil {
			errorCount++
		} else {
			errorCount = 0
		}

		// TODO(henrikhodne): Make this configurable
		sleepTime := c.Duration("refresh-interval") * time.Duration(errorCount+1)
		if sleepTime > 5*time.Minute {
			sleepTime = 5 * time.Minute
		}

		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"err":          err,
				"backoff_time": sleepTime,
			}).Error("an error occurred when refreshing")
		}

		time.Sleep(sleepTime)
	}

}
