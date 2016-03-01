package http

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/net/context"

	"github.com/travis-ci/cloud-brain/cloudbrain"
)

// Handler returns an http.Handler for the API.
func Handler(ctx context.Context, core *cloudbrain.Core) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/instances/", handleInstances(ctx, core))
	mux.Handle("/instances", handleInstances(ctx, core))

	return mux
}

func parseRequest(r *http.Request, out interface{}) error {
	err := json.NewDecoder(r.Body).Decode(out)
	if err != nil && err != io.EOF {
		return fmt.Errorf("Failed to parse JSON input: %s", err)
	}
	return err
}

func respondError(w http.ResponseWriter, status int, err error) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := &ErrorResponse{Errors: make([]string, 0, 1)}
	if err != nil {
		resp.Errors = append(resp.Errors, err.Error())
	}

	json.NewEncoder(w).Encode(resp)
}

func respondOk(w http.ResponseWriter, body interface{}) {
	w.Header().Add("Content-Type", "application/json")

	if body == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(body)
	}
}

type ErrorResponse struct {
	Errors []string `json:"errors"`
}
