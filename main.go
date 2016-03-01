package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/garyburd/redigo/redis"
	_ "github.com/lib/pq"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	cbhttp "github.com/travis-ci/cloud-brain/http"
	"github.com/travis-ci/cloud-brain/worker"
	"golang.org/x/net/context"
)

func main() {
	ctx := context.Background()
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	if os.Getenv("REDIS_URL") == "" {
		logrus.Fatal("REDIS_URL env var must be provided")
	}
	redisPool := &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 3 * time.Minute,
		Dial: func() (redis.Conn, error) {
			return redis.DialURL(os.Getenv("REDIS_URL"))
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			_, err := c.Do("PING")
			return err
		},
	}
	wb := worker.NewRedisWorker(redisPool, "cloud-brain:worker")

	if os.Getenv("DATABASE_URL") == "" {
		logrus.Fatal("DATABASE_URL env var must be provided")
	}
	pgdb, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		logrus.Fatalf("error connecting to DB: %v", err)
	}
	db := database.NewPostgresDB(pgdb)
	provider := &cloud.FakeProvider{}

	core := cloudbrain.NewCore(&cloudbrain.CoreConfig{
		CloudProvider: provider,
		DB:            db,
		WorkerBackend: wb,
	})

	go func() {
		err := worker.Run(ctx, "create", wb, worker.WorkerFunc(core.ProviderCreateInstance))
		logrus.WithField("err", err).Error("create worker crashed")
	}()

	go runRefresh(ctx, core)

	log.Fatal(http.ListenAndServe(":6060", cbhttp.Handler(ctx, core)))
}

func runRefresh(ctx context.Context, core *cloudbrain.Core) {
	var errorCount uint
	for {
		err := core.ProviderRefresh(ctx)
		if err != nil {
			errorCount++
		} else {
			errorCount = 0
		}

		// TODO(henrikhodne): Make this configurable
		sleepTime := 1 * time.Duration(errorCount+1) * time.Second
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
