package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"text/template"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/mitchellh/multistep"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"golang.org/x/net/context"
)

var gceStartupScript = template.Must(template.New("gce-startup").Parse(`#!/usr/bin/env bash
{{ if .AutoImplode }}echo poweroff | at now + {{ .AutoImplodeMinutes }} minutes{{ end }}
cat > ~travis/.ssh/authorized_keys <<EOF
{{ .SSHPubKey }}
EOF
`))

func init() {
	registerProvider("gce", "Google Compute Engine", NewGCEProviderFromJSON)
}

// GCEProvider is an implementation of cloud.Provider backed by Google Compute
// Engine.
type GCEProvider struct {
	client         *compute.Service
	projectID      string
	imageProjectID string
	ic             *gceInstanceConfig
}

type gceStartContext struct {
	id               string
	instChan         chan Instance
	errChan          chan error
	createAttrs      CreateAttributes
	image            *compute.Image
	script           string
	bootStart        time.Time
	instance         *compute.Instance
	instanceInsertOp *compute.Operation
}

type gceStopContext struct {
	id               string
	instChan         chan Instance
	errChan          chan error
	terminateStart   time.Time
	instanceConfig   gceInstanceConfig
	instance         *compute.Instance
	instanceDeleteOp *compute.Operation
	ctx              context.Context
}

type gceInstanceConfig struct {
	MachineType        *compute.MachineType
	PremiumMachineType *compute.MachineType
	Zone               *compute.Zone
	Network            *compute.Network
	DiskType           string
	DiskSize           int64
	HardTimeoutMinutes int64
	AutoImplode        bool
	Preemptible        bool
	SkipStopPoll       bool
	StopPrePollSleep   time.Duration
	StopPollSleep      time.Duration
}

// GCEAccountJSON represents the JSON key file received from GCE when creating a
// new key for a service account.
type GCEAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

// GCEProviderConfiguration contains all the configuration needed to create a
// GCEProvider.
type GCEProviderConfiguration struct {
	AccountJSON         GCEAccountJSON `json:"account_json"`
	ProjectID           string         `json:"project_id"`
	ImageProjectID      string         `json:"image_project_id"`
	Zone                string         `json:"zone"`
	StandardMachineType string         `json:"standard_machine_type"`
	PremiumMachineType  string         `json:"premium_machine_type"`
	Network             string         `json:"network"`
	DiskSize            int64          `json:"disk_size"`
	AutoImplodeTime     time.Duration  `json:"auto_implode_time"`
	AutoImplode         bool           `json:"auto_implode"`
	Preemptible         bool           `json:"preemptible"`
}

type gceStartupScriptInfo struct {
	AutoImplode        bool
	AutoImplodeMinutes int64
	SSHPubKey          string
}

// NewGCEProviderFromJSON deserializes the given jsonConfig into a
// GCEProviderConfiguration and creates a GCEProvider from that. Used to
// register the provider with the registry.
func NewGCEProviderFromJSON(jsonConfig []byte) (Provider, error) {
	var config GCEProviderConfiguration
	err := json.Unmarshal(jsonConfig, &config)
	if err != nil {
		return nil, err
	}

	return NewGCEProvider(config)
}

// NewGCEProvider creates a new GCEProvider with the given configuration.
func NewGCEProvider(conf GCEProviderConfiguration) (*GCEProvider, error) {
	clientConfig := &jwt.Config{
		Email:      conf.AccountJSON.ClientEmail,
		PrivateKey: []byte(conf.AccountJSON.PrivateKey),
		Scopes: []string{
			compute.DevstorageFullControlScope,
			compute.ComputeScope,
		},
		TokenURL: conf.AccountJSON.TokenURI,
	}

	client, err := compute.New(clientConfig.Client(oauth2.NoContext))
	if err != nil {
		return nil, err
	}

	zone, err := client.Zones.Get(conf.ProjectID, conf.Zone).Do()
	if err != nil {
		return nil, err
	}

	machineType, err := client.MachineTypes.Get(conf.ProjectID, zone.Name, conf.StandardMachineType).Do()
	if err != nil {
		return nil, err
	}

	premiumMachineType, err := client.MachineTypes.Get(conf.ProjectID, zone.Name, conf.PremiumMachineType).Do()
	if err != nil {
		return nil, err
	}

	network, err := client.Networks.Get(conf.ProjectID, conf.Network).Do()
	if err != nil {
		return nil, err
	}

	return &GCEProvider{
		client:         client,
		projectID:      conf.ProjectID,
		imageProjectID: conf.ImageProjectID,
		ic: &gceInstanceConfig{
			Preemptible:        conf.Preemptible,
			DiskSize:           conf.DiskSize,
			DiskType:           fmt.Sprintf("zones/%s/diskTypes/pd-ssd", zone.Name),
			MachineType:        machineType,
			PremiumMachineType: premiumMachineType,
			AutoImplode:        conf.AutoImplode,
			HardTimeoutMinutes: int64(conf.AutoImplodeTime.Minutes()),
			Zone:               zone,
			Network:            network,
		},
	}, nil
}

// List returns a list of all instances on Google Compute Engine that was\
// created by Cloud Brain.
func (p *GCEProvider) List() ([]Instance, error) {
	instanceList, err := p.client.Instances.List(p.projectID, p.ic.Zone.Name).Filter("name eq ^testing-gce-.+").Do()
	if err != nil {
		return nil, err
	}

	var instances []Instance

	for _, gceInstance := range instanceList.Items {
		instance := Instance{
			ID: strings.TrimPrefix(gceInstance.Name, "testing-gce-"),
		}

		for _, ni := range gceInstance.NetworkInterfaces {
			if ni.AccessConfigs == nil {
				continue
			}

			for _, ac := range ni.AccessConfigs {
				if ac.NatIP != "" {
					instance.IPAddress = ac.NatIP
					break
				}
			}
		}

		switch gceInstance.Status {
		case "PROVISIONING", "STAGING":
			instance.State = InstanceStateStarting
		case "RUNNING":
			instance.State = InstanceStateRunning
		case "STOPPING":
			instance.State = InstanceStateTerminating
		case "TERMINATED":
			instance.State = InstanceStateTerminated
		}

		instances = append(instances, instance)
	}

	return instances, nil
}

// Create creates a new instance with the given ID and using the given
// attributes.
func (p *GCEProvider) Create(id string, attr CreateAttributes) (Instance, error) {
	state := &multistep.BasicStateBag{}

	c := &gceStartContext{
		id:          id,
		createAttrs: attr,
		instChan:    make(chan Instance),
		errChan:     make(chan error),
	}

	runner := &multistep.BasicRunner{
		Steps: []multistep.Step{
			&gceStartMultistepWrapper{c: c, f: p.stepGetImage},
			&gceStartMultistepWrapper{c: c, f: p.stepRenderScript},
			&gceStartMultistepWrapper{c: c, f: p.stepInsertInstance},
		},
	}

	abandonedStart := false
	defer func(c *gceStartContext) {
		if c.instance != nil && abandonedStart {
			// TODO(henrikhodne): Can we remove this, or queue a delete job instead?
			_, _ = p.client.Instances.Delete(p.projectID, p.ic.Zone.Name, c.instance.Name).Do()
		}
	}(c)

	go runner.Run(state)

	select {
	case inst := <-c.instChan:
		return inst, nil
	case err := <-c.errChan:
		abandonedStart = true
		return Instance{}, err
	}
}

// Remove stops the instance with the given ID and terminates it.
func (p *GCEProvider) Remove(id string) (Instance, error) {
	state := &multistep.BasicStateBag{}

	c := &gceStopContext{
		id:       id,
		instChan: make(chan Instance),
		errChan:  make(chan error),
	}

	runner := &multistep.BasicRunner{
		Steps: []multistep.Step{
			&gceStopMultistepWrapper{c: c, f: p.stepDeleteInstance},
		},
	}

	go runner.Run(state)

	select {
	case inst := <-c.instChan:
		return inst, nil
	case err := <-c.errChan:
		return Instance{}, err
	}
}

func (p *GCEProvider) stepDeleteInstance(c *gceStopContext) multistep.StepAction {
	op, err := p.client.Instances.Delete(p.projectID, p.ic.Zone.Name, c.instance.Name).Do()
	if err != nil {
		cbcontext.LoggerFromContext(c.ctx).WithField("err", err).Error("error deleting instance")
		c.errChan <- err
		return multistep.ActionHalt
	}

	c.instanceDeleteOp = op
	return multistep.ActionContinue
}

func (p *GCEProvider) stepGetImage(c *gceStartContext) multistep.StepAction {
	images, err := p.client.Images.List(p.imageProjectID).Filter(fmt.Sprintf("name eq ^%s", c.createAttrs.ImageName)).Do()
	if err != nil {
		c.errChan <- err
		return multistep.ActionHalt
	}

	if len(images.Items) == 0 {
		c.errChan <- fmt.Errorf("no image found with name %s", c.createAttrs.ImageName)
		return multistep.ActionHalt
	}

	imagesByName := map[string]*compute.Image{}
	imageNames := []string{}
	for _, image := range images.Items {
		imagesByName[image.Name] = image
		imageNames = append(imageNames, image.Name)
	}

	sort.Strings(imageNames)

	c.image = imagesByName[imageNames[len(imageNames)-1]]
	return multistep.ActionContinue
}

func (p *GCEProvider) stepRenderScript(c *gceStartContext) multistep.StepAction {
	var scriptBuf bytes.Buffer
	err := gceStartupScript.Execute(&scriptBuf, gceStartupScriptInfo{
		AutoImplode:        p.ic.AutoImplode,
		AutoImplodeMinutes: p.ic.HardTimeoutMinutes,
		SSHPubKey:          c.createAttrs.PublicSSHKey,
	})
	if err != nil {
		c.errChan <- err
		return multistep.ActionHalt
	}

	c.script = scriptBuf.String()
	return multistep.ActionContinue
}

func (p *GCEProvider) stepInsertInstance(c *gceStartContext) multistep.StepAction {
	inst := p.buildInstance(c.id, c.createAttrs, c.image.SelfLink, c.script)

	c.bootStart = time.Now().UTC()

	op, err := p.client.Instances.Insert(p.projectID, p.ic.Zone.Name, inst).Do()
	if err != nil {
		c.errChan <- err
		return multistep.ActionHalt
	}

	c.instance = inst
	c.instanceInsertOp = op

	c.instChan <- Instance{
		ID:    c.id,
		State: InstanceStateStarting,
	}
	return multistep.ActionContinue
}

func (p *GCEProvider) buildInstance(id string, createAttrs CreateAttributes, imageLink, startupScript string) *compute.Instance {
	var machineType *compute.MachineType
	switch createAttrs.InstanceType {
	case InstanceTypePremium:
		machineType = p.ic.PremiumMachineType
	default:
		machineType = p.ic.MachineType
	}

	return &compute.Instance{
		Description: "Travis CI test VM",
		Disks: []*compute.AttachedDisk{
			&compute.AttachedDisk{
				Type:       "PERSISTENT",
				Mode:       "READ_WRITE",
				Boot:       true,
				AutoDelete: true,
				InitializeParams: &compute.AttachedDiskInitializeParams{
					SourceImage: imageLink,
					DiskType:    p.ic.DiskType,
					DiskSizeGb:  p.ic.DiskSize,
				},
			},
		},
		Scheduling: &compute.Scheduling{
			Preemptible: p.ic.Preemptible,
		},
		MachineType: machineType.SelfLink,
		Name:        fmt.Sprintf("testing-gce-%s", id),
		Metadata: &compute.Metadata{
			Items: []*compute.MetadataItems{
				&compute.MetadataItems{
					Key:   "startup-script",
					Value: googleapi.String(startupScript),
				},
			},
		},
		NetworkInterfaces: []*compute.NetworkInterface{
			&compute.NetworkInterface{
				AccessConfigs: []*compute.AccessConfig{
					&compute.AccessConfig{
						Name: "AccessConfig brought to you by cloud-brain",
						Type: "ONE_TO_ONE_NAT",
					},
				},
				Network: p.ic.Network.SelfLink,
			},
		},
		ServiceAccounts: []*compute.ServiceAccount{
			&compute.ServiceAccount{
				Email: "default",
				Scopes: []string{
					"https://www.googleapis.com/auth/userinfo.email",
					compute.DevstorageFullControlScope,
					compute.ComputeScope,
				},
			},
		},
		Tags: &compute.Tags{
			Items: []string{
				"testing",
			},
		},
	}
}

// Get retrieves information about the instance with the given name from Google
// Compute Engine. Return ErrInstanceNotFound if an instance with the given ID
// wasn't found, or some other error if we were unable to get information about
// the instance.
func (p *GCEProvider) Get(id string) (Instance, error) {
	gceInstance, err := p.client.Instances.Get(p.projectID, p.ic.Zone.Name, fmt.Sprintf("testing-gce-%s", id)).Do()
	if err != nil {
		if gceErr, ok := err.(*googleapi.Error); ok && gceErr.Code == http.StatusNotFound {
			return Instance{}, ErrInstanceNotFound
		}
		return Instance{}, err
	}

	instance := Instance{
		ID: id,
	}

	for _, ni := range gceInstance.NetworkInterfaces {
		if ni.AccessConfigs == nil {
			continue
		}

		for _, ac := range ni.AccessConfigs {
			if ac.NatIP != "" {
				instance.IPAddress = ac.NatIP
				break
			}
		}
	}

	switch gceInstance.Status {
	case "PROVISIONING", "STAGING":
		instance.State = InstanceStateStarting
	case "RUNNING":
		instance.State = InstanceStateRunning
	case "STOPPING":
		instance.State = InstanceStateTerminating
	case "TERMINATED":
		instance.State = InstanceStateTerminated
	}

	return instance, nil
}

// Destroy terminates and removes the instance with the given ID. Returns
// ErrInstanceNotFound if an instance with the given ID wasn't found, or some
// other error if another error occurred. Does not wait for the instance to
// terminate, will return as soon as the destroy job is enqueued with GCE.
func (p *GCEProvider) Destroy(id string) error {
	_, err := p.client.Instances.Delete(p.projectID, p.ic.Zone.Name, fmt.Sprintf("testing-gce-%s", id)).Do()
	if err != nil {
		if gceErr, ok := err.(*googleapi.Error); ok && gceErr.Code == http.StatusNotFound {
			return ErrInstanceNotFound
		}
	}

	return err
}

type gceStartMultistepWrapper struct {
	f func(*gceStartContext) multistep.StepAction
	c *gceStartContext
}

type gceStopMultistepWrapper struct {
	f func(*gceStopContext) multistep.StepAction
	c *gceStopContext
}

func (gismw *gceStartMultistepWrapper) Run(multistep.StateBag) multistep.StepAction {
	return gismw.f(gismw.c)
}

func (gismw *gceStartMultistepWrapper) Cleanup(multistep.StateBag) { return }

func (gismw *gceStopMultistepWrapper) Run(multistep.StateBag) multistep.StepAction {
	return gismw.f(gismw.c)
}

func (gismw *gceStopMultistepWrapper) Cleanup(multistep.StateBag) { return }
