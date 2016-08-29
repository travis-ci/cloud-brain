package main

import (
	"database/sql"
	"fmt"
	"net/http"
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
	cbhttp "github.com/travis-ci/cloud-brain/http"
	"gopkg.in/urfave/cli.v2"
)

func main() {
	app := &cli.App{
		Name:      "cloudbrain-http",
		Version:   cloudbrain.VersionString,
		Copyright: cloudbrain.CopyrightString,
		Usage:     "Run the HTTP server part of Cloud Brain",
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
				Name:  "addr",
				Usage: "host:port to listen to",
				Value: func() string {
					v := ":" + os.Getenv("PORT")
					if v == ":" {
						v = ":42191"
					}
					return v
				}(),
				EnvVars: []string{"CLOUDBRAIN_ADDR"},
			},
			&cli.StringSliceFlag{
				Name:    "auth-token",
				Usage:   "authentication token(s) to accept",
				EnvVars: []string{"CLOUDBRAIN_AUTH_TOKEN"},
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
	backgroundBackend := background.NewRedisBackend(redisPool, c.String("redis-worker-prefix"))
	err := backgroundBackend.WaitForConnection()
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("background backend creation failed")
	}

	if c.String("database-url") == "" {
		cbcontext.LoggerFromContext(ctx).Fatal("database-url flag is required")
	}
	pgdb, err := sql.Open("postgres", c.String("database-url"))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("couldn't connect to postgres")
	}
	db := database.NewPostgresDB([32]byte{}, pgdb)

	core := cloudbrain.NewCore(db, backgroundBackend)

	err = http.ListenAndServe(c.String("addr"), cbhttp.Handler(ctx, core, c.StringSlice("auth-token")))
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("ListenAndServe returned error")
	}
	return nil
}
