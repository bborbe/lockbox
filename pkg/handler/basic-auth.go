// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"crypto/subtle"
	"net/http"
)

// NewBasicAuth wraps delegate with HTTP Basic authentication, matching the
// TeamVault contract: a missing or wrong credential yields 401 with a
// WWW-Authenticate challenge. Credentials are compared in constant time.
func NewBasicAuth(username, password string, delegate http.Handler) http.Handler {
	wantUser := []byte(username)
	wantPass := []byte(password)
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		user, pass, ok := req.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(user), wantUser) == 1
		passOK := subtle.ConstantTimeCompare([]byte(pass), wantPass) == 1
		if !ok || !userOK || !passOK {
			resp.Header().Set("WWW-Authenticate", `Basic realm="lockbox"`)
			http.Error(resp, "unauthorized", http.StatusUnauthorized)
			return
		}
		delegate.ServeHTTP(resp, req)
	})
}
