package cloudbrain

import (
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/Sirupsen/logrus"
	"github.com/hashicorp/go-multierror"
	"github.com/pborman/uuid"
	"github.com/pkg/errors"
	"github.com/travis-ci/cloud-brain/background"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloud"
	"github.com/travis-ci/cloud-brain/database"
	"golang.org/x/crypto/scrypt"
	"golang.org/x/net/context"
	"gopkg.in/urfave/cli.v2"
)

var (
	//VersionString gets set during `make`
	VersionString = "?"
	//RevisionString gets set during `make`
	RevisionString = "?"
	//RevisionURLString gets set during `make`
	RevisionURLString = "?"
	//GeneratedString gets set during `make`
	GeneratedString = "?"
	//CopyrightString gets set during `make`
	CopyrightString = "?"
)

func init() {
	cli.VersionPrinter = customVersionPrinter
	_ = os.Setenv("VERSION", VersionString)
	_ = os.Setenv("REVISION", RevisionString)
	_ = os.Setenv("GENERATED", GeneratedString)
}

func customVersionPrinter(c *cli.Context) {
	fmt.Printf("%v v=%v rev=%v d=%v\n", filepath.Base(c.App.Name),
		VersionString, RevisionString, GeneratedString)
}

// MaxCreateRetries is the number of times the "create" job will be retried.
const MaxCreateRetries = 10

// Core is used as a central manager for all Cloud Brain functionality. The HTTP
// API and the background workers are just frontends for the Core, and calls
// methods on Core for functionality.
type Core struct {
	db database.DB
	bb background.Backend

	cloudProvidersMutex sync.Mutex
	cloudProviders      map[string]cloud.Provider
}

// NewCore is used to create a new Core backed by the given database and
// background Backend.
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
		UpstreamID:   instance.UpstreamID,
		ErrorReason:  instance.ErrorReason,
	}, nil
}

// CreateInstanceAttributes contains attributes needed to start an instance
type CreateInstanceAttributes struct {
	ImageName    string
	InstanceType string
	PublicSSHKey string
}

//DeleteInstanceAttributes contains attributes needed to delete an instance
type DeleteInstanceAttributes struct {
	InstanceID string
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
		return nil, errors.Wrap(err, "error creating instance in database")
	}

	err = c.bb.Enqueue(background.Job{
		UUID:       uuid.New(),
		Context:    ctx,
		Payload:    []byte(id),
		Queue:      "create",
		MaxRetries: MaxCreateRetries,
	})
	if err != nil {
		// TODO(henrikhodne): Delete the record in the database?
		return nil, errors.Wrap(err, "error enqueueing 'create' job in the background")
	}

	return &Instance{
		ID:           id,
		ProviderName: providerName,
		Image:        attr.ImageName,
		State:        "creating",
	}, nil
}

// RemoveInstance creates an instance in the database and queues off the cloud
// create job in the background.
func (c *Core) RemoveInstance(ctx context.Context, attr DeleteInstanceAttributes) error {
	inst, err := c.db.GetInstance(attr.InstanceID)
	if err != nil {
		return errors.Wrap(err, "error fetching instance from DB")
	}

	if inst.State == "terminating" || inst.State == "terminated" {
		return errors.Wrapf(err, "not removing instance, state is already %s", inst.State)
	}

	err = c.bb.Enqueue(background.Job{
		UUID:       uuid.New(),
		Context:    ctx,
		Payload:    []byte(attr.InstanceID),
		Queue:      "remove",
		MaxRetries: MaxCreateRetries,
	})
	if err != nil {
		return errors.Wrap(err, "error enqueueing 'remove' job in the background")
	}

	return nil
}

// ProviderCreateInstance is used to schedule the creation of the instance with
// the given ID on the provider selected for that instance.
func (c *Core) ProviderCreateInstance(ctx context.Context, byteID []byte) error {
	id := string(byteID)

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
	}).Info("creating instance")

	dbInstance, err := c.db.GetInstance(id)
	if err != nil {
		return errors.Wrap(err, "error fetching instance from DB")
	}

	cloudProvider, err := c.cloudProvider(dbInstance.ProviderName)
	if err != nil {
		return errors.Wrapf(err, "couldn't find provider with given name: %v", dbInstance.ProviderName)
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

		dbInstance.State = "errored"
		dbInstance.ErrorReason = err.Error()

		err = c.db.UpdateInstance(dbInstance)
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"err":         err,
				"instance_id": id,
				"provider_id": instance.ID,
				"upstream_id": instance.UpstreamID,
			}).Error("couldn't update instance in DB")
			return err
		}

		return err
	}

	dbInstance.State = "starting"

	err = c.db.UpdateInstance(dbInstance)
	if err != nil {
		return errors.Wrap(err, "couldn't update instance in DB")
	}

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
		"provider_id": instance.ID,
	}).Info("created instance")

	return nil
}

// ProviderRemoveInstance is used to schedule the creation of the instance with
// the given ID on the provider selected for that instance.
func (c *Core) ProviderRemoveInstance(ctx context.Context, byteID []byte) error {
	id := string(byteID)

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
	}).Info("removing instance")

	dbInstance, err := c.db.GetInstance(id)
	if err != nil {
		return errors.Wrap(err, "error fetching instance from DB")
	}

	cloudProvider, err := c.cloudProvider(dbInstance.ProviderName)
	if err != nil {
		return errors.Wrapf(err, "couldn't find provider with given name: %v", dbInstance.ProviderName)
	}

	err = cloudProvider.Destroy(id)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err":         err,
			"instance_id": id,
		}).Error("error removing instance")

		return err
	}

	dbInstance.State = "terminating"
	err = c.db.UpdateInstance(dbInstance)
	if err != nil {
		return errors.Wrap(err, "error updating instance state to terminating in DB")
	}

	cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
		"instance_id": id,
	}).Info("removed instance")

	return nil
}

// ProviderRefresh is used to synchronize the data on all the cloud providers
// with the data in our database.
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

		seenIds := make(map[string]bool)

		for _, instance := range instances {
			seenIds[instance.ID] = true

			dbInstance, err := c.db.GetInstance(instance.ID)
			if err != nil {
				cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
					"err":           err,
					"provider_name": providerName,
					"provider_id":   instance.ID,
				}).Error("failed fetching instance from database")
				continue
			}

			dbInstance.State = string(instance.State)
			dbInstance.IPAddress = instance.IPAddress
			dbInstance.UpstreamID = instance.UpstreamID
			dbInstance.ErrorReason = instance.ErrorReason

			err = c.db.UpdateInstance(dbInstance)
			if err != nil {
				cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
					"err":         err,
					"provider":    providerName,
					"provider_id": instance.ID,
					"db_id":       dbInstance.ID,
				}).Error("failed to update instance in database")
			}
		}

		terminatingDbInstances, err := c.db.GetInstancesByState("terminating")
		if err != nil {
			cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
				"err":      err,
				"provider": providerName,
			}).Error("failed to fetch list of terminating instances")
			continue
		}

		// find instances that were in terminating state
		// if the id is no longer in seenIds, it was deleted
		// from GCE, we can now consider it terminated
		for _, dbInstance := range terminatingDbInstances {
			_, found := seenIds[dbInstance.ID]
			if !found {
				dbInstance.State = "terminated"

				err = c.db.UpdateInstance(dbInstance)
				if err != nil {
					cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
						"err":      err,
						"provider": providerName,
						"db_id":    dbInstance.ID,
					}).Error("failed to update instance in database")
				}
			}
		}

		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"provider":       providerName,
			"instance_count": len(instances),
		}).Info("refreshed instances")
	}

	return result
}

// CheckToken is used to check whether a given tokenID+token is in the database.
// Returns (true, nil) iff the token is valid, (false, nil) if the token is
// invalid, and (false, err) if an error occurred while fetching the token.
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

// cloudProvider is used to get the provider implementation for the cloud
// provider with a given name. Return an error if no cloud provider with the
// given name exists, or if an error occurred refreshing the configuration from
// the database.
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

// refreshProviders is used to regenerate the c.cloudProviders map with the
// configurations stored in the database.
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

// Instance is a single compute instance.
type Instance struct {
	ID           string
	ProviderName string
	Image        string
	State        string
	IPAddress    string
	UpstreamID   string
	ErrorReason  string
}
