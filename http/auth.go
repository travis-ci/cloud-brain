package http

import (
	"fmt"
	"net/http"
	"strings"
)

type authWrapper struct {
	authTokens []string
	handler    http.Handler
}

func (aw *authWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	prefix := "token "
	if !strings.HasPrefix(r.Header.Get("Authorization"), prefix) {
		respondError(w, http.StatusUnauthorized, fmt.Errorf("Authorization header required"))
		return
	}

	actualToken := r.Header.Get("Authorization")[len(prefix):]

	for _, token := range aw.authTokens {
		if token == actualToken {
			aw.handler.ServeHTTP(w, r)
			return
		}
	}

	respondError(w, http.StatusUnauthorized, fmt.Errorf("invalid token"))
}
