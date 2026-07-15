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
	// Name is the human-readable secret name; may be empty.
	Name            string `json:"name"`
	Username        string `json:"username"`
	URL             string `json:"url"`
	CurrentRevision string `json:"current_revision"`
}

// RevisionData is the body of GET /api/secret-revisions/{key}/data.
type RevisionData struct {
	Password string `json:"password"`
	File     string `json:"file"`
}

// SearchResults is the body of GET /api/secrets/?search=q. It is a single
// page: count is the number of matches, next and previous are always null
// (Lockbox does not paginate), results is the array of matches.
type SearchResults struct {
	Count    int            `json:"count"`
	Next     *string        `json:"next"`
	Previous *string        `json:"previous"`
	Results  []SearchResult `json:"results"`
}

// SearchResult is one entry in a SearchResults page. It carries the secret's
// metadata (name, username, url) plus its hashid and the absolute api_url of
// its metadata endpoint. It carries no secret value.
type SearchResult struct {
	Hashid   string `json:"hashid"`
	APIURL   string `json:"api_url"`
	Name     string `json:"name"`
	Username string `json:"username"`
	URL      string `json:"url"`
}
