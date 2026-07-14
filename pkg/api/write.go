// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package api

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/bborbe/errors"
	libhttp "github.com/bborbe/http"

	"github.com/bborbe/lockbox/pkg/secret"
)

// CreateSecretRequest is the decoded body of POST /api/secrets/ (create) and
// PATCH /api/secrets/{hashid}/ (update).  All fields except content_type are
// optional.  secret_data is write-only: it is never echoed in any API response.
type CreateSecretRequest struct {
	// ContentType selects the secret kind.  Must be "password" or "file".
	ContentType string `json:"content_type"`
	// Name is the human-readable secret name.
	Name string `json:"name"`
	// Username is the account name associated with the secret (optional).
	Username string `json:"username"`
	// URL is the resource the secret grants access to (optional).
	URL string `json:"url"`
	// Description is a free-text description of the secret (optional).
	Description string `json:"description"`
	// SecretData is the write-only secret value block.  Required for creates.
	// OtpKeyData and Filename are accepted but NOT persisted (spec Desired
	// Behavior 1: otp_key_data accepted but not stored; only current value is
	// kept).
	SecretData *SecretData `json:"secret_data"`
}

// SecretData is the polymorphic secret value block sent by TeamVault clients.
// The fields present depend on content_type:
//
//	content_type "password"  → password (required), otp_key_data (accepted, ignored)
//	content_type "file"      → file_content (required, base64), filename (accepted, ignored)
type SecretData struct {
	// Password is the secret value for a password secret.
	Password string `json:"password"`
	// OtpKeyData is accepted for TeamVault compatibility but NOT stored.
	OtpKeyData string `json:"otp_key_data"`
	// FileContent is the base64-encoded file payload for a file secret.
	FileContent string `json:"file_content"`
	// Filename is the optional original filename for a file secret (accepted but
	// not persisted).
	Filename string `json:"filename"`
}

// SecretRepresentation is the TeamVault-shaped response body returned by
// POST /api/secrets/ (201) and PATCH /api/secrets/{hashid}/ (200).  It does
// NOT include secret_data (write-only) and does NOT include otp, shares,
// access_policy, or status (spec Non-goals).
type SecretRepresentation struct {
	// Hashid is the short lookup key for the secret (e.g. "MOPmQL").
	Hashid string `json:"hashid"`
	// APIURL is the absolute URL of the secret's metadata endpoint.
	APIURL string `json:"api_url"`
	// ContentType is the secret kind ("password" or "file").
	ContentType string `json:"content_type"`
	// Name is the human-readable secret name.
	Name string `json:"name"`
	// Username is the account name associated with the secret.
	Username string `json:"username"`
	// URL is the resource the secret grants access to.
	URL string `json:"url"`
	// Description is the free-text description of the secret.
	Description string `json:"description"`
}

// Validate checks a CreateSecretRequest against the TeamVault write contract
// and returns the secret.Secret it maps to.  On any contract violation it
// returns an error already wrapped with HTTP 400 via libhttp.WrapWithStatusCode,
// so the JSON error handler emits a 400.
//
// Validation rules:
//
//	(a) ContentType is required.
//	(b) ContentType must be "password" or "file".
//	(c) SecretData is required.
//	(d) For "password": SecretData.Password is required.
//	(e) For "file":    SecretData.FileContent is required and must be valid base64.
func (r CreateSecretRequest) Validate(ctx context.Context) (secret.Secret, error) {
	// (a) content_type required
	if r.ContentType == "" {
		return secret.Secret{}, libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "content_type is required"),
			http.StatusBadRequest,
		)
	}
	// (b) content_type must be "password" or "file"
	if r.ContentType != secret.ContentTypePassword && r.ContentType != secret.ContentTypeFile {
		return secret.Secret{}, libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "unsupported content_type %q", r.ContentType),
			http.StatusBadRequest,
		)
	}
	// (c) secret_data required
	if r.SecretData == nil {
		return secret.Secret{}, libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "secret_data is required"),
			http.StatusBadRequest,
		)
	}

	password, file, err := validateSecretData(ctx, r.ContentType, r.SecretData)
	if err != nil {
		return secret.Secret{}, err
	}

	return secret.Secret{
		Name:        r.Name,
		Description: r.Description,
		Username:    r.Username,
		URL:         r.URL,
		ContentType: r.ContentType,
		Password:    password,
		File:        file,
	}, nil
}

// validateSecretData validates the secret_data block for the given content type
// and returns the (password, file, error) tuple.  It factors the per-type
// rules so both Validate and ApplyUpdate share the same logic.
func validateSecretData(
	ctx context.Context,
	contentType string,
	data *SecretData,
) (password, file string, err error) {
	switch contentType {
	case secret.ContentTypePassword:
		if data.Password == "" {
			return "", "", libhttp.WrapWithStatusCode(
				errors.Errorf(ctx, "secret_data.password is required for a password secret"),
				http.StatusBadRequest,
			)
		}
		return data.Password, "", nil

	case secret.ContentTypeFile:
		if data.FileContent == "" {
			return "", "", libhttp.WrapWithStatusCode(
				errors.Errorf(ctx, "secret_data.file_content is required for a file secret"),
				http.StatusBadRequest,
			)
		}
		// Verify valid base64
		if _, decodeErr := base64.StdEncoding.DecodeString(data.FileContent); decodeErr != nil {
			return "", "", libhttp.WrapWithStatusCode(
				errors.Errorf(ctx, "secret_data.file_content is not valid base64"),
				http.StatusBadRequest,
			)
		}
		// Store the original base64 string (revision-data read echoes the base64 form)
		return "", data.FileContent, nil

	default:
		// Should be unreachable because the caller already validated contentType,
		// but return a 400 just in case.
		return "", "", libhttp.WrapWithStatusCode(
			errors.Errorf(ctx, "unsupported content_type %q", contentType),
			http.StatusBadRequest,
		)
	}
}

// ApplyUpdate returns a copy of existing with the metadata fields present in r
// overlaid, and — when r.SecretData is non-nil — the value replaced.
// content_type is immutable: r.ContentType is ignored.  secret_data is
// validated against existing.ContentType (not r.ContentType).  Returns a 400
// (via libhttp.WrapWithStatusCode) if a present secret_data is malformed for
// the existing content type.
func (r CreateSecretRequest) ApplyUpdate(
	ctx context.Context,
	existing secret.Secret,
) (secret.Secret, error) {
	result := existing

	// Overlay metadata fields that are present (non-empty) in the request.
	// Empty string fields are left unchanged (matches "updates the fields
	// present in the body"; empty-string clears are out of scope).
	if r.Name != "" {
		result.Name = r.Name
	}
	if r.Description != "" {
		result.Description = r.Description
	}
	if r.Username != "" {
		result.Username = r.Username
	}
	if r.URL != "" {
		result.URL = r.URL
	}

	// content_type is immutable — ignore r.ContentType; preserve existing.
	// No change to result.ContentType.

	// If secret_data is present, validate it against the existing content type
	// and replace the value fields.
	if r.SecretData != nil {
		password, file, err := validateSecretData(ctx, existing.ContentType, r.SecretData)
		if err != nil {
			return secret.Secret{}, err
		}
		result.Password = password
		result.File = file
	}

	return result, nil
}
