package main

import (
	"log"
	"net/http"
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/cloudbrain"
	"github.com/travis-ci/cloud-brain/database"
	cbhttp "github.com/travis-ci/cloud-brain/http"
	"github.com/travis-ci/cloud-brain/worker"
)

func main() {
	ctx := context.Background()
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	db := database.NewMemoryDatabase()
	provider := &cloud.FakeProvider{}
	mw := worker.NewMemoryWorker()

	core := cloudbrain.NewCore(&cloudbrain.CoreConfig{
		CloudProvider: provider,
		DB:            db,
		WorkerBackend: mw,
	})

	go worker.Run(ctx, "create", mw, worker.WorkerFunc(core.ProviderCreateInstance))

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
