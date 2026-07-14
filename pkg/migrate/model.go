// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package migrate implements a one-shot API-to-API importer that reads all
// secrets from a running TeamVault instance and writes them into a running
// Lockbox instance via their respective HTTP APIs.
package migrate

// TeamVaultSecret is one entry of GET /api/secrets/ as returned by TeamVault's
// SecretSerializer. secret_data is write_only on TeamVault and therefore never
// present here; the actual value must be fetched from CurrentRevision's data
// endpoint.
type TeamVaultSecret struct {
	// Hashid is the short, stable identifier TeamVault uses to address the
	// secret (used as the Lockbox key on migration).
	Hashid string `json:"hashid"`
	// Username is the account name associated with the secret; may be empty.
	Username string `json:"username"`
	// URL is the resource the secret grants access to; may be empty.
	URL string `json:"url"`
	// ContentType is one of "password", "cc" (credit card) or "file".
	ContentType string `json:"content_type"`
	// CurrentRevision is the absolute URL of the secret's current
	// SecretRevision detail endpoint (GET .../api/secret-revisions/{hashid}/).
	// Appending "data" yields the endpoint that returns the decrypted value.
	CurrentRevision string `json:"current_revision"`
	// DataReadable reports whether the authenticated user is allowed to read
	// this secret's revision data.
	DataReadable bool `json:"data_readable"`
	// Name is the human-readable secret name.
	Name string `json:"name"`
	// Status is one of "ok", "needs_changing" or "deleted".
	Status string `json:"status"`
}

// teamVaultListResponse is the DRF-paginated envelope returned by
// GET /api/secrets/.
type teamVaultListResponse struct {
	Count    int               `json:"count"`
	Next     *string           `json:"next"`
	Previous *string           `json:"previous"`
	Results  []TeamVaultSecret `json:"results"`
}

// ContentTypePassword identifies a password secret in TeamVaultSecret.ContentType.
const ContentTypePassword = "password"

// ContentTypeCreditCard identifies a credit-card secret in
// TeamVaultSecret.ContentType. Lockbox has no field for credit-card data, so
// these secrets are always skipped during migration.
const ContentTypeCreditCard = "cc"

// ContentTypeFile identifies a file secret in TeamVaultSecret.ContentType.
const ContentTypeFile = "file"
