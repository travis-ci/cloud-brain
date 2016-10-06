package http

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/cloudbrain"
)

var (
	errAuthorizationHeaderRequired = fmt.Errorf("authorization header required")
	errInvalidToken                = fmt.Errorf("invalid token")
	errNonNumericalTokenID         = fmt.Errorf("invalid token (token ID must be numerical)")
	errFetchingToken               = fmt.Errorf("error fetching token")
)

// Handler returns an http.Handler for the API.
func Handler(ctx context.Context, core *cloudbrain.Core, authTokens []string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/instances/", handleInstances(ctx, core))
	mux.Handle("/instances", handleInstances(ctx, core))

	return &authWrapper{
		core:    core,
		handler: mux,
		ctx:     ctx,
	}
}

func parseRequest(ctx context.Context, r *http.Request, out interface{}) error {
	err := json.NewDecoder(r.Body).Decode(out)
	if err != nil && err != io.EOF {
		cbcontext.LoggerFromContext(ctx).WithField("err", err).Info("couldn't parse request")
		return fmt.Errorf("Failed to parse JSON input: %s", err)
	}
	return err
}

func respondError(ctx context.Context, w http.ResponseWriter, status int, err error) {
	cbcontext.LoggerFromContext(ctx).WithField("response", status).WithField("err", err).Info()

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := &ErrorResponse{Errors: make([]string, 0, 1)}
	if err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	json.NewEncoder(w).Encode(resp)
}

func respondOk(ctx context.Context, w http.ResponseWriter, body interface{}) {
	var status int
	if body == nil {
		status = http.StatusNoContent
	} else {
		status = http.StatusOK
	}

	respondStatus(ctx, w, status, body)
}

func respondStatus(ctx context.Context, w http.ResponseWriter, status int, body interface{}) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)

	if body != nil {
		json.NewEncoder(w).Encode(body)
	}

	cbcontext.LoggerFromContext(ctx).WithField("response", status).Info()
}

// An ErrorResponse is returned by the HTTP API when an error occurs.
type ErrorResponse struct {
	Errors []string `json:"errors"`
}
