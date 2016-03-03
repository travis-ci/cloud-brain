package http

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/travis-ci/cloud-brain/cloudbrain"
)

type authWrapper struct {
	core    *cloudbrain.Core
	handler http.Handler
}

func (aw *authWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "token "
	if !strings.HasPrefix(r.Header.Get("Authorization"), prefix) {
		respondError(w, http.StatusUnauthorized, fmt.Errorf("Authorization header required"))
		return
	}

	actualToken := r.Header.Get("Authorization")[len(prefix):]
	components := strings.Split(actualToken, "-")
	if len(components) != 2 {
		respondError(w, http.StatusUnauthorized, fmt.Errorf("invalid token (should be using format \"id-token\")"))
		return
	}

	tokenID, err := strconv.ParseUint(components[0], 10, 64)
	if err != nil {
		respondError(w, http.StatusUnauthorized, fmt.Errorf("invalid token (token ID must be numerical)"))
		return
	}

	validToken, err := aw.core.CheckToken(tokenID, components[1])
	if err != nil {
		// TODO(henrikhodne): Log error
		respondError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}

	if !validToken {
		respondError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
		return
	}

	aw.handler.ServeHTTP(w, r)
}
