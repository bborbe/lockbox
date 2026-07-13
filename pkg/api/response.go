// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package api defines the wire DTOs for the TeamVault-compatible HTTP API.
// The json tags mirror the exact field names the real TeamVault server (and the
// teamvault-cli cmd/fakevault reference server) emit, so existing clients work
// against Lockbox with only a base-URL change.
package api

// SecretMetadata is the body of GET /api/secrets/{key}/.
// current_revision is an absolute URL pointing at the revision-data endpoint.
type SecretMetadata struct {
	Username        string `json:"username"`
	URL             string `json:"url"`
	CurrentRevision string `json:"current_revision"`
}

// RevisionData is the body of GET /api/secret-revisions/{key}/data.
type RevisionData struct {
	Password string `json:"password"`
	File     string `json:"file"`
}

// SearchResults is the body of GET /api/secrets/?search=q.
type SearchResults struct {
	Results []SearchResult `json:"results"`
}

// SearchResult is one entry in a SearchResults list; api_url is the absolute
// URL of that secret's metadata endpoint.
type SearchResult struct {
	APIURL string `json:"api_url"`
}

// UpsertRequest is the body of PUT /api/secrets/{key}/ — creates or replaces
// the secret stored under key.
type UpsertRequest struct {
	Username string `json:"username"`
	URL      string `json:"url"`
	Password string `json:"password"`
	File     string `json:"file"`
}

// UpsertResult is the body returned by PUT /api/secrets/{key}/ on success; the
// api_url points at the secret's metadata endpoint (same shape as SearchResult
// so clients can treat both uniformly).
type UpsertResult struct {
	APIURL string `json:"api_url"`
}
