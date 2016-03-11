package cloudbrain

import (
	"crypto/subtle"
	"encoding/hex"

	"github.com/Sirupsen/logrus"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/worker"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/net/context"
)

const MaxCreateRetries = 10

type Core struct {
	cloud cloud.Provider
	db    database.DB
	wb    worker.Backend
}

// TODO(henrikhodne): Is this necessary? Why not just make a Core directly?
type CoreConfig struct {
	DB            database.DB
	WorkerBackend worker.Backend
}

func NewCore(conf *CoreConfig) (*Core, error) {
	cloudProviders, err := conf.DB.ListProviders()
	if err != nil {
		return nil, err
	}

	cloudProvider, err := cloud.NewProvider(cloudProviders[0].Type, cloudProviders[0].Config)
	if err != nil {
		return nil, err
	}

	return &Core{
		cloud: cloudProvider,
		db:    conf.DB,
		wb:    conf.WorkerBackend,
	}, nil
}

// GetInstance gets the instance information stored in the database for a given
// instance ID.
func (c *Core) GetInstance(ctx context.Context, id string) (*Instance, error) {
	instance, err := c.db.GetInstance(id)

	if err == database.ErrInstanceNotFound {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return &Instance{
		ID:           instance.ID,
		ProviderName: instance.ProviderName,
		Image:        instance.Image,
		State:        instance.State,
		IPAddress:    instance.IPAddress,
	}, nil
}

// CreateInstanceAttributes contains attributes needed to start an instance
type CreateInstanceAttributes struct {
	ImageName    string
	InstanceType string
	PublicSSHKey string
}

// CreateInstance creates an instance in the database and queues off the cloud
// create job in the background.
func (c *Core) CreateInstance(ctx context.Context, providerName string, attr CreateInstanceAttributes) (*Instance, error) {
	id, err := c.db.CreateInstance(database.Instance{
		ProviderName: providerName,
		Image:        attr.ImageName,
		InstanceType: attr.InstanceType,
		PublicSSHKey: attr.PublicSSHKey,
		State:        "creating",
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":        err,
			"provider":   providerName,
			"image_name": attr.ImageName,
		}).Error("error creating instance in database")
		return nil, err
	}

	err = c.wb.Enqueue(worker.Job{
		Context:    ctx,
		Payload:    []byte(id),
		Queue:      "create",
		MaxRetries: MaxCreateRetries,
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error enqueueing 'create' job in the background")
		// TODO(henrikhodne): Delete the record in the database?
		return nil, err
	}

	return &Instance{
		ID:           id,
		ProviderName: providerName,
		Image:        attr.ImageName,
		State:        "creating",
	}, nil
}

func (c *Core) ProviderCreateInstance(ctx context.Context, byteID []byte) error {
	id := string(byteID)

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
	}).Info("creating instance")

	dbInstance, err := c.db.GetInstance(id)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error fetching instance from DB")
		return err
	}

	instance, err := c.cloud.Create(id, cloud.CreateAttributes{
		ImageName:    dbInstance.Image,
		InstanceType: cloud.InstanceType(dbInstance.InstanceType),
		PublicSSHKey: dbInstance.PublicSSHKey,
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error creating instance")
		return err
	}

	dbInstance.State = "starting"

	err = c.db.UpdateInstance(dbInstance)
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

func (c *Core) ProviderRefresh(ctx context.Context) error {
	instances, err := c.cloud.List()
	if err != nil {
		return err
	}

	for _, instance := range instances {
		dbInstance, err := c.db.GetInstance(instance.ID)
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"provider":    c.cloud.Name(),
				"provider_id": instance.ID,
			}).Error("failed fetching instance from database")
			continue
		}

		dbInstance.IPAddress = instance.IPAddress
		dbInstance.State = string(instance.State)

		err = c.db.UpdateInstance(dbInstance)
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"provider":    c.cloud.Name(),
				"provider_id": instance.ID,
				"db_id":       dbInstance.ID,
			}).Error("failed to update instance in database")
		}
	}

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"provider":       c.cloud.Name(),
		"instance_count": len(instances),
	}).Info("refreshed instances")

	return nil
}

func (c *Core) CheckToken(tokenID uint64, token string) (bool, error) {
	salt, hash, err := c.db.GetSaltAndHashForTokenID(tokenID)
	if err != nil {
		return false, err
	}

	decodedToken, err := hex.DecodeString(token)
	if err != nil {
		return false, err
	}

	generatedHash, err := scrypt.Key(decodedToken, salt, 16384, 8, 1, 32)
	if err != nil {
		return false, err
	}

	return subtle.ConstantTimeCompare(generatedHash, hash) == 1, nil
}

type Instance struct {
	ID           string
	ProviderName string
	Image        string
	State        string
	IPAddress    string
}
