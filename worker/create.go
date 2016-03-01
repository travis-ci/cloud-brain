package worker

import (
	"golang.org/x/net/context"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/provider"
)

type CreateWorker struct {
	Provider provider.Provider
	DB       database.DB
}

func (w *CreateWorker) Work(ctx context.Context, byteID []byte) error {
	id := string(byteID)

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
	}).Info("creating instance")

	dbInstance, err := w.DB.GetInstance(id)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error fetching instance from DB")
		return err
	}

	instance, err := w.Provider.Create(dbInstance.Image)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error creating instance")
		return err
	}

	dbInstance.ProviderID = instance.ID
	dbInstance.State = "starting"

	err = w.DB.UpdateInstance(dbInstance)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
			"provider_id": instance.ID,
		}).Error("couldn't update instance in DB")
		return err
	}

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
		"provider_id": instance.ID,
	}).Info("created instance")

	return nil
}
