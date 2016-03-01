// Package server implements the HTTP server part of Cloud Brain
package server

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/gorilla/mux"
	"github.com/travis-ci/cloud-brain/database"
	"github.com/travis-ci/cloud-brain/worker"
)

type instancePayload struct {
	ID        string  `json:"id"`
	Provider  string  `json:"provider"`
	Image     string  `json:"image"`
	IPAddress *string `json:"ip_address"`
	State     string  `json:"state"`
}

func RunServer(ctx context.Context, db database.DB, w worker.Backend) {
	s := &server{
		ctx: ctx,
		db:  db,
		w:   w,
	}
	s.run()
}

type server struct {
	ctx context.Context
	db  database.DB
	w   worker.Backend
}

func (s *server) run() {
	r := mux.NewRouter()
	r.HandleFunc("/instances/{id}", s.handleInstancesRetrieve).Methods("GET")
	r.HandleFunc("/instances", s.handleInstancesCreate).Methods("POST")
	http.Handle("/", r)
}
