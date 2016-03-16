package cloudbrain

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/hashicorp/go-multierror"
	"github.com/travis-ci/cloud-brain/background"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/database"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/net/context"
)

const MaxCreateRetries = 10

type Core struct {
	db database.DB
	bb background.Backend

	cloudProvidersMutex sync.Mutex
	cloudProviders      map[string]cloud.Provider
}

func NewCore(db database.DB, bb background.Backend) *Core {
	return &Core{
		db: db,
		bb: bb,
	}
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

	err = c.bb.Enqueue(background.Job{
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

	cloudProvider, err := c.cloudProvider(dbInstance.ProviderName)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"instance_id":   id,
			"provider_name": dbInstance.ProviderName,
			"err":           err,
		}).Error("couldn't find provider with given name")
		return err
	}

	instance, err := cloudProvider.Create(id, cloud.CreateAttributes{
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
	c.refreshProviders()
	c.cloudProvidersMutex.Lock()
	defer c.cloudProvidersMutex.Unlock()

	var result error

	for providerName, cloudProvider := range c.cloudProviders {
		instances, err := cloudProvider.List()
		if err != nil {
			result = multierror.Append(result, err)
			continue
		}

		for _, instance := range instances {
			dbInstance, err := c.db.GetInstance(instance.ID)
			if err != nil {
				cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
					"provider_name": providerName,
					"provider_id":   instance.ID,
				}).Error("failed fetching instance from database")
				continue
			}

			dbInstance.IPAddress = instance.IPAddress
			dbInstance.State = string(instance.State)

			err = c.db.UpdateInstance(dbInstance)
			if err != nil {
				cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
					"provider":    providerName,
					"provider_id": instance.ID,
					"db_id":       dbInstance.ID,
				}).Error("failed to update instance in database")
			}
		}

		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"provider":       providerName,
			"instance_count": len(instances),
		}).Info("refreshed instances")
	}

	return result
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

func (c *Core) cloudProvider(name string) (cloud.Provider, error) {
	c.cloudProvidersMutex.Lock()
	cloudProvider, ok := c.cloudProviders[name]
	c.cloudProvidersMutex.Unlock()
	if !ok {
		err := c.refreshProviders()
		if err != nil {
			return nil, err
		}

		c.cloudProvidersMutex.Lock()
		cloudProvider, ok = c.cloudProviders[name]
		c.cloudProvidersMutex.Unlock()
		if !ok {
			// This really shouldn't happen, since the database should ensure
			// that a provider with a matching name exists.
			return nil, fmt.Errorf("couldn't find a provider with that name")
		}
	}

	return cloudProvider, nil
}

func (c *Core) refreshProviders() error {
	c.cloudProvidersMutex.Lock()
	defer c.cloudProvidersMutex.Unlock()
	dbCloudProviders, err := c.db.ListProviders()
	if err != nil {
		return err
	}

	cloudProviders := make(map[string]cloud.Provider)

	for _, dbCloudProvider := range dbCloudProviders {
		cloudProvider, err := cloud.NewProvider(dbCloudProvider.Type, dbCloudProvider.Config)
		if err != nil {
			return err
		}

		cloudProviders[dbCloudProvider.Name] = cloudProvider
	}

	c.cloudProviders = cloudProviders

	return nil
}

type Instance struct {
	ID           string
	ProviderName string
	Image        string
	State        string
	IPAddress    string
}
