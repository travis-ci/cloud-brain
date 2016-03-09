package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/jwt"

	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"

	"github.com/mitchellh/multistep"
	"github.com/pborman/uuid"
)

var gceStartupScript = template.Must(template.New("gce-startup").Parse(`#!/usr/bin/env bash
{{ if .AutoImplode }}echo poweroff | at now + {{ .AutoImplodeMinutes }} minutes{{ end }}
cat > ~travis/.ssh/authorized_keys <<EOF
{{ .SSHPubKey }}
EOF
`))

type GCEProvider struct {
	client         *compute.Service
	projectID      string
	imageProjectID string
	ic             *gceInstanceConfig
}

type gceStartContext struct {
	instChan         chan Instance
	errChan          chan error
	createAttrs      CreateAttributes
	image            *compute.Image
	script           string
	bootStart        time.Time
	instance         *compute.Instance
	instanceInsertOp *compute.Operation
}

type gceInstanceConfig struct {
	MachineType        *compute.MachineType
	PremiumMachineType *compute.MachineType
	Zone               *compute.Zone
	Network            *compute.Network
	DiskType           string
	DiskSize           int64
	AutoImplode        bool
	HardTimeoutMinutes int64
	Preemptible        bool
}

type gceAccountJSON struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

type GCEProviderConfiguration struct {
	AccountJSON         string
	ProjectID           string
	ImageProjectID      string
	Zone                string
	StandardMachineType string
	PremiumMachineType  string
	Network             string
	DiskSize            int64
	AutoImplode         bool
	AutoImplodeTime     time.Duration
	Preemptible         bool
}

type gceStartupScriptInfo struct {
	AutoImplode        bool
	AutoImplodeMinutes int64
	SSHPubKey          string
}

func NewGCEProvider(conf GCEProviderConfiguration) (*GCEProvider, error) {
	a, err := loadGoogleAccountJSON(conf.AccountJSON)
	if err != nil {
		return nil, err
	}

	clientConfig := &jwt.Config{
		Email:      a.ClientEmail,
		PrivateKey: []byte(a.PrivateKey),
		Scopes: []string{
			compute.DevstorageFullControlScope,
			compute.ComputeScope,
		},
		TokenURL: "https://accounts.google.com/o/oauth2/token",
	}

	client, err := compute.New(clientConfig.Client(oauth2.NoContext))
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
			AutoImplode:        conf.AutoImplode,
			HardTimeoutMinutes: int64(conf.AutoImplodeTime.Minutes()),
		},
	}, nil
}

func loadGoogleAccountJSON(filenameOrJSON string) (*gceAccountJSON, error) {
	var (
		reader io.Reader
		err    error
	)

	if strings.HasPrefix(strings.TrimSpace(filenameOrJSON), "{") {
		reader = bytes.NewReader([]byte(filenameOrJSON))
	} else {
		var file *os.File
		file, err = os.Open(filenameOrJSON)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		reader = file
	}

	a := &gceAccountJSON{}
	err = json.NewDecoder(reader).Decode(a)
	return a, err
}

func (p *GCEProvider) Name() string {
	return "gce"
}

func (p *GCEProvider) List() ([]Instance, error) {
	return nil, nil
}

func (p *GCEProvider) Create(attr CreateAttributes) (Instance, error) {
	state := &multistep.BasicStateBag{}

	c := &gceStartContext{
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
	inst := p.buildInstance(c.createAttrs, c.image.SelfLink, c.script)

	c.bootStart = time.Now().UTC()

	op, err := p.client.Instances.Insert(p.projectID, p.ic.Zone.Name, inst).Do()
	if err != nil {
		c.errChan <- err
		return multistep.ActionHalt
	}

	c.instance = inst
	c.instanceInsertOp = op

	c.instChan <- Instance{
		ID:    inst.Name,
		State: InstanceStateStarting,
	}
	return multistep.ActionContinue
}

func (p *GCEProvider) buildInstance(createAttrs CreateAttributes, imageLink, startupScript string) *compute.Instance {
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
		Name:        fmt.Sprintf("testing-gce-%s", uuid.NewRandom()),
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
						Name: "AccessConfig brought to you by travis-worker",
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

func (p *GCEProvider) Get(providerID string) (Instance, error) {
	return Instance{}, nil
}

func (p *GCEProvider) Destroy(providerID string) error {
	return nil
}

type gceStartMultistepWrapper struct {
	f func(*gceStartContext) multistep.StepAction
	c *gceStartContext
}

func (gismw *gceStartMultistepWrapper) Run(multistep.StateBag) multistep.StepAction {
	return gismw.f(gismw.c)
}

func (gismw *gceStartMultistepWrapper) Cleanup(multistep.StateBag) { return }
