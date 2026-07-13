// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

// NewSecretSearchHandler serves GET /api/secrets/?search=q — the keys matching
// query, each as an absolute api_url to its metadata endpoint.
func NewSecretSearchHandler(store secret.Store) libhttp.WithError {
	return libhttp.NewJSONHandler(libhttp.JSONHandlerFunc(
		func(ctx context.Context, req *http.Request) (any, error) {
			query := req.URL.Query().Get("search")
			keys, err := store.Search(ctx, query)
			if err != nil {
				return nil, errors.Wrapf(ctx, err, "search secrets for %q failed", query)
			}
			prefix := apiPrefix(req, "secrets/")
			results := make([]api.SearchResult, 0, len(keys))
			for _, key := range keys {
				results = append(results, api.SearchResult{
					APIURL: absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
				})
			}
			return api.SearchResults{Results: results}, nil
		},
	))
}
