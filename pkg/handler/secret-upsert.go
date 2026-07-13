// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	"github.com/gorilla/mux"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

// NewSecretUpsertHandler serves PUT /api/secrets/{key}/ — creates or replaces
// the secret stored under key from a JSON body of username, url, password and
// file, and returns the absolute URL of the secret's metadata endpoint.
func NewSecretUpsertHandler(store secret.Store) libhttp.WithError {
	return libhttp.NewJSONHandler(libhttp.JSONHandlerFunc(
		func(ctx context.Context, req *http.Request) (any, error) {
			var body api.UpsertRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, errors.Wrap(ctx, err, "decode upsert request failed")
			}
			key := secret.Key(mux.Vars(req)["key"])
			value := secret.Secret{
				Username: body.Username,
				URL:      body.URL,
				Password: body.Password,
				File:     body.File,
			}
			if err := store.Upsert(ctx, key, value); err != nil {
				return nil, errors.Wrapf(ctx, err, "upsert secret %s failed", key)
			}
			prefix := apiPrefix(req, "secrets/"+key.String()+"/")
			return api.UpsertResult{
				APIURL: absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
			}, nil
		},
	))
}
