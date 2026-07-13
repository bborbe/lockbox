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

// NewRevisionDataHandler serves GET /api/secret-revisions/{key}/data — the
// secret's actual payload (password, file).
func NewRevisionDataHandler(store secret.Store) libhttp.WithError {
	return libhttp.NewJSONHandler(libhttp.JSONHandlerFunc(
		func(ctx context.Context, req *http.Request) (any, error) {
			key := secret.Key(mux.Vars(req)["key"])
			found, err := store.Get(ctx, key)
			if err != nil {
				return nil, errors.Wrapf(ctx, err, "get secret %s failed", key)
			}
			return api.RevisionData{
				Password: found.Password,
				File:     found.File,
			}, nil
		},
	))
}
