// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"
	"github.com/gorilla/mux"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

// NewSecretMetadataHandler serves GET /api/secrets/{key}/ — the secret metadata
// (username, url) plus an absolute current_revision URL pointing at the
// revision-data endpoint on the same API prefix.
func NewSecretMetadataHandler(store secret.Store) libhttp.WithError {
	return libhttp.NewJSONHandler(libhttp.JSONHandlerFunc(
		func(ctx context.Context, req *http.Request) (any, error) {
			key := secret.Key(mux.Vars(req)["key"])
			found, err := store.Get(ctx, key)
			if err != nil {
				return nil, errors.Wrapf(ctx, err, "get secret %s failed", key)
			}
			prefix := apiPrefix(req, "secrets/"+key.String()+"/")
			revision := absoluteURL(req, prefix+"secret-revisions/"+key.String()+"/data")
			return api.SecretMetadata{
				Username:        found.Username,
				URL:             found.URL,
				CurrentRevision: revision,
			}, nil
		},
	))
}
