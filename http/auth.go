package http

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/travis-ci/cloud-brain/cloudbrain"
	"golang.org/x/net/context"
)

type authWrapper struct {
	core    *cloudbrain.Core
	handler http.Handler
	ctx     context.Context
}

func (aw *authWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "token "
	if !strings.HasPrefix(r.Header.Get("Authorization"), prefix) {
		respondError(aw.ctx, w, http.StatusUnauthorized, errAuthorizationHeaderRequired)
		return
	}

	actualToken := r.Header.Get("Authorization")[len(prefix):]
	components := strings.Split(actualToken, "-")
	if len(components) != 2 {
		respondError(aw.ctx, w, http.StatusUnauthorized, errNonNumericalTokenID)
		return
	}

	tokenID, err := strconv.ParseUint(components[0], 10, 64)
	if err != nil {
		respondError(aw.ctx, w, http.StatusUnauthorized, errNonNumericalTokenID)
		return
	}

	validToken, err := aw.core.CheckToken(tokenID, components[1])
	if err != nil {
		respondError(aw.ctx, w, http.StatusUnauthorized, errInvalidToken)
		return
	}

	if !validToken {
		respondError(aw.ctx, w, http.StatusUnauthorized, errInvalidToken)
		return
	}

	aw.handler.ServeHTTP(w, r)
}
