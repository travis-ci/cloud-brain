package main

import (
	"log"
	"net/http"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/provider"
	"github.com/travis-ci/cloud-brain/server"
	"github.com/travis-ci/cloud-brain/worker"
)

func main() {
	ctx := context.Background()
	logrus.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	db := database.NewMemoryDatabase()
	provider := &provider.FakeProvider{}

	mw := worker.NewMemoryWorker()
	go worker.Run(ctx, "create", mw, &worker.CreateWorker{
		Provider: provider,
		DB:       db,
	})

	cw := &worker.RefreshWorker{
		ProviderName: "fake",
		Provider:     provider,
		DB:           db,
	}
	go cw.Run(ctx)

	server.RunServer(ctx, db, mw)

	log.Fatal(http.ListenAndServe(":6060", nil))
}
