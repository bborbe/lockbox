// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"net/http"
	"strings"
)

// requestScheme reports the external scheme of req, honouring a terminating
// reverse proxy's X-Forwarded-Proto before falling back to the connection.
func requestScheme(req *http.Request) string {
	if proto := req.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}
	if req.TLS != nil {
		return "https"
	}
	return "http"
}

// apiPrefix returns the request path up to and including the mount prefix by
// trimming suffix off the current path — e.g. for path "/api/v1/secrets/AbC/"
// and suffix "secrets/AbC/" it returns "/api/v1/". This keeps generated URLs on
// the same prefix (`/api/` or `/api/v1/`) the client called.
func apiPrefix(req *http.Request, suffix string) string {
	return strings.TrimSuffix(req.URL.Path, suffix)
}

// absoluteURL builds scheme://host + path for the given request.
func absoluteURL(req *http.Request, path string) string {
	return requestScheme(req) + "://" + req.Host + path
}
