package worker

import (
	"time"

	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/provider"
)

type RefreshWorker struct {
	ProviderName string
	Provider     provider.Provider
	DB           database.DB

	errorCount uint
}

func (w *RefreshWorker) Run(ctx context.Context) error {
	for {
		err := w.refresh(ctx)
		if err != nil {
			w.errorCount++
		} else {
			w.errorCount = 0
		}

		// TODO(henrikhodne): Make this configurable
		sleepTime := 1 * time.Duration(w.errorCount+1) * time.Second
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

func (w *RefreshWorker) refresh(ctx context.Context) error {
	instances, err := w.Provider.List()
	if err != nil {
		return err
	}

	for _, instance := range instances {
		dbInstance, err := w.DB.GetInstanceByProviderID(w.ProviderName, instance.ID)
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"provider":    w.ProviderName,
				"provider_id": instance.ID,
			}).Error("failed fetching instance from database")
			continue
		}

		dbInstance.IPAddress = instance.IPAddress
		dbInstance.State = string(instance.State)

		err = w.DB.UpdateInstance(dbInstance)
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"provider":    w.ProviderName,
				"provider_id": instance.ID,
				"db_id":       dbInstance.ID,
			}).Error("failed to update instance in database")
		}
	}

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"provider":       w.ProviderName,
		"instance_count": len(instances),
	}).Debug("refreshed instances")

	return nil
}
