package main

import (
	"database/sql"
	"os"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/garyburd/redigo/redis"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
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
			Name:   "gce-account-json",
			Usage:  "The GCE account JSON blob, or a path pointing to the JSON blob",
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
		cli.DurationFlag{
			Name:   "refresh-interval",
			Usage:  "The interval at which to refresh the cached instances",
			Value:  5 * time.Second,
			EnvVar: "CLOUDBRAIN_REFRESH_INTERVAL",
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
	db := database.NewPostgresDB(pgdb)

	provider, err := cloud.NewGCEProvider(cloud.GCEProviderConfiguration{
		AccountJSON:         c.String("gce-account-json"),
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
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Fatal("couldn't create GCE provider")
	}

	core := cloudbrain.NewCore(&cloudbrain.CoreConfig{
		CloudProvider: provider,
		DB:            db,
		WorkerBackend: workerBackend,
	})

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
