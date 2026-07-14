// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package handler

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/lockbox/pkg/api"
	"github.com/bborbe/lockbox/pkg/secret"
)

// NewSecretCreateHandler serves POST /api/secrets/ — the TeamVault create
// endpoint. It decodes a TeamVault create body, validates it, generates a
// fresh unique key, stores the secret encrypted, and responds 201 with a
// TeamVault-shaped representation containing the new hashid and api_url.
func NewSecretCreateHandler(store secret.Store, keyGen secret.KeyGenerator) libhttp.WithError {
	return libhttp.WithErrorFunc(
		func(ctx context.Context, resp http.ResponseWriter, req *http.Request) error {
			body, err := decodeBody(ctx, req)
			if err != nil {
				return err
			}

			sec, err := body.Validate(ctx)
			if err != nil {
				return err
			}

			key, err := generateUniqueKey(ctx, store, keyGen, sec)
			if err != nil {
				return err
			}

			return respondWithRepresentation(ctx, resp, req, key, sec)
		},
	)
}

func decodeBody(ctx context.Context, req *http.Request) (api.CreateSecretRequest, error) {
	var body api.CreateSecretRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		return api.CreateSecretRequest{}, libhttp.WrapWithStatusCode(
			errors.Wrap(ctx, err, "decode create request failed"),
			http.StatusBadRequest,
		)
	}
	return body, nil
}

func generateUniqueKey(
	ctx context.Context,
	store secret.Store,
	keyGen secret.KeyGenerator,
	sec secret.Secret,
) (secret.Key, error) {
	const maxKeyAttempts = 10
	for attempt := 0; attempt < maxKeyAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", errors.Wrap(ctx, err, "context cancelled during key generation")
		}
		candidate, err := keyGen.Generate(ctx)
		if err != nil {
			return "", errors.Wrap(ctx, err, "generate key failed")
		}
		if err := store.Create(ctx, candidate, sec); err != nil {
			if stderrors.Is(err, secret.ErrKeyExists) {
				continue
			}
			return "", errors.Wrapf(ctx, err, "create secret %s failed", candidate)
		}
		return candidate, nil
	}
	return "", errors.Errorf(
		ctx,
		"could not generate a unique key after %d attempts",
		maxKeyAttempts,
	)
}

func respondWithRepresentation(
	ctx context.Context,
	resp http.ResponseWriter,
	req *http.Request,
	key secret.Key,
	sec secret.Secret,
) error {
	prefix := apiPrefix(req, "secrets/")
	repr := api.SecretRepresentation{
		Hashid:      key.String(),
		APIURL:      absoluteURL(req, prefix+"secrets/"+key.String()+"/"),
		ContentType: sec.ContentType,
		Name:        sec.Name,
		Username:    sec.Username,
		URL:         sec.URL,
		Description: sec.Description,
	}
	return libhttp.SendJSONResponse(ctx, resp, repr, http.StatusCreated)
}
