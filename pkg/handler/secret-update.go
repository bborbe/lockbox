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

// NewSecretUpdateHandler serves PATCH /api/secrets/{hashid}/ — the TeamVault
// update endpoint. It merges the metadata fields present in the body and,
// when secret_data is present, the new value into the stored secret, keeps
// content_type immutable, and responds 200 with the updated representation.
func NewSecretUpdateHandler(store secret.Store) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			key := secret.Key(mux.Vars(req)["key"])

			// Load the existing secret. The store's Get returns an error for a
			// missing key, so we map any Get error to 404 on PATCH (the handler
			// cannot distinguish between "key absent" and "store read failed"
			// without a separate sentinel; both cases require a 404 response).
			existing, err := store.Get(ctx, key)
			if err != nil {
				return libhttp.WrapWithStatusCode(
					errors.Wrapf(ctx, err, "secret %s not found", key),
					http.StatusNotFound,
				)
			}

			// Decode the JSON body into a CreateSecretRequest.
			var body api.CreateSecretRequest
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return libhttp.WrapWithStatusCode(
					errors.Wrap(ctx, err, "decode update request failed"),
					http.StatusBadRequest,
				)
			}

			// ApplyUpdate merges the metadata fields present in body and, when
			// secret_data is non-nil, replaces the secret value. It validates
			// secret_data against the existing content_type and returns a 400
			// (via WrapWithStatusCode) on malformed data. content_type from the
			// body is ignored — it is immutable on update.
			updated, err := body.ApplyUpdate(ctx, *existing)
			if err != nil {
				return err // already wrapped with 400 by ApplyUpdate
			}

			// Persist the updated secret. The key already exists, so Upsert
			// (overwrite-in-place) is correct here.
			if err := store.Upsert(ctx, key, updated); err != nil {
				return errors.Wrapf(ctx, err, "update secret %s failed", key)
			}

			// Build the representation and respond 200.
			prefix := apiPrefix(req, "secrets/"+key.String()+"/")
			repr := api.SecretRepresentation{
				Hashid:      key.String(),
				APIURL:      absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
				ContentType: updated.ContentType,
				Name:        updated.Name,
				Username:    updated.Username,
				URL:         updated.URL,
				Description: updated.Description,
			}
			return libhttp.SendJSONResponse(ctx, resp, repr, http.StatusOK)
		},
	)
}
