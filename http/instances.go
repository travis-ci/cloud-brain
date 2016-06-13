package http

import (
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloudbrain"
)

func handleInstances(ctx context.Context, core *cloudbrain.Core) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = cbcontext.FromRequestID(ctx, r.Header.Get("X-Request-ID"))

		switch r.Method {
		case "GET":
			handleInstancesGet(ctx, core, w, r)
		case "POST":
			handleInstancesPost(ctx, core, w, r)
		default:
			respondError(w, http.StatusMethodNotAllowed, nil)
		}
	})
}

func handleInstancesGet(ctx context.Context, core *cloudbrain.Core, w http.ResponseWriter, r *http.Request) {
	// Determine the path...
	prefix := "/instances/"
	if !strings.HasPrefix(r.URL.Path, prefix) {
		respondError(w, http.StatusNotFound, nil)
		return
	}
	path := r.URL.Path[len(prefix):]
	if path == "" {
		respondError(w, http.StatusNotFound, nil)
		return
	}

	instance, err := core.GetInstance(ctx, path)
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Error("couldn't get instance")
		respondError(w, http.StatusInternalServerError, nil)
		return
	}
	if instance == nil {
		respondError(w, http.StatusNotFound, nil)
		return
	}

	respondOk(w, instanceToResponse(instance))
}

func handleInstancesPost(ctx context.Context, core *cloudbrain.Core, w http.ResponseWriter, r *http.Request) {
	var req CreateInstanceRequest

	if err := parseRequest(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, err)
		return
	}

	instance, err := core.CreateInstance(ctx, req.Provider, cloudbrain.CreateInstanceAttributes{
		ImageName:    req.Image,
		InstanceType: req.InstanceType,
		PublicSSHKey: req.PublicSSHKey,
	})
	if err != nil {
		respondError(w, http.StatusInternalServerError, err)
		return
	}

	respondOk(w, instanceToResponse(instance))
}

func instanceToResponse(instance *cloudbrain.Instance) *InstanceResponse {
	body := &InstanceResponse{
		ID:           instance.ID,
		ProviderName: instance.ProviderName,
		Image:        instance.Image,
		State:        instance.State,
	}
	if instance.IPAddress != "" {
		body.IPAddress = &instance.IPAddress
	}

	return body
}

// An InstanceResponse is returned by the HTTP API that contains information
// about an instance.
type InstanceResponse struct {
	ID           string  `json:"id"`
	ProviderName string  `json:"provider"`
	Image        string  `json:"image"`
	IPAddress    *string `json:"ip_address"`
	State        string  `json:"state"`
}

// CreateInstanceRequest contains the data in the request body for a create
// instance request.
type CreateInstanceRequest struct {
	Provider     string `json:"provider"`
	Image        string `json:"image"`
	InstanceType string `json:"instance_type"`
	PublicSSHKey string `json:"public_ssh_key"`
}
