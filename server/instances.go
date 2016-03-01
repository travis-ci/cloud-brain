package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Sirupsen/logrus"
	"github.com/gorilla/mux"
	"github.com/travis-ci/cloud-brain/cbcontext"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/worker"
)

// MaxCreateRetries is the number of times the background "create" job will be
// retried before giving up.
const MaxCreateRetries = 10

type serializedInstance struct {
	ID        string  `json:"id"`
	Provider  string  `json:"provider"`
	Image     string  `json:"image"`
	IPAddress *string `json:"ip_address"`
	State     string  `json:"state"`
}

type errorReply struct {
	Error string `json:"error"`
}

func (s *server) handleInstancesRetrieve(w http.ResponseWriter, req *http.Request) {
	ctx := s.ctx
	if req.Header.Get("X-Request-ID") != "" {
		ctx = cbcontext.FromUUID(ctx, req.Header.Get("X-Request-ID"))
	}

	vars := mux.Vars(req)
	id := vars["id"]

	instance, err := s.db.GetInstance(id)
	if err == database.ErrInstanceNotFound {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(errorReply{Error: "No instance with that ID"})

		return
	}
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err": err,
		}).Error("error getting instance from database")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorReply{Error: "An error occurred retrieving the instance"})

		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	inst := serializedInstance{
		ID:       instance.ID,
		Provider: instance.Provider,
		Image:    instance.Image,
		State:    instance.State,
	}
	if instance.IPAddress != "" {
		inst.IPAddress = &instance.IPAddress
	}

	json.NewEncoder(w).Encode(inst)
}

func (s *server) handleInstancesCreate(w http.ResponseWriter, req *http.Request) {
	ctx := s.ctx
	if req.Header.Get("X-Request-ID") != "" {
		ctx = cbcontext.FromUUID(ctx, req.Header.Get("X-Request-ID"))
	}

	var parsedInstance serializedInstance
	err := json.NewDecoder(req.Body).Decode(&parsedInstance)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(errorReply{Error: "Problems parsing JSON"})

		return
	}

	id, err := s.db.CreateInstance(database.Instance{
		Provider: parsedInstance.Provider,
		Image:    parsedInstance.Image,
		State:    "creating",
	})
	if err != nil {
		cbcontext.LoggerFromContext(ctx).WithFields(logrus.Fields{
			"err": err,
		}).Error("error creating instance")

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(errorReply{Error: "An error occurred creating the instance"})

		return
	}

	s.w.Enqueue(worker.Job{
		Context:    ctx,
		Payload:    []byte(id),
		Queue:      "create",
		MaxRetries: MaxCreateRetries,
	})

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Location", fmt.Sprintf("/instances/%s", id))
	w.WriteHeader(http.StatusCreated)

	instance := parsedInstance
	instance.ID = id
	instance.State = "creating"
	json.NewEncoder(w).Encode(instance)
}
