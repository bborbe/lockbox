// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package secret defines the Lockbox domain model: a secret identified by a
// lookup Key, holding the metadata and current-revision data that the
// TeamVault-compatible API exposes.
package secret

// Key is the lookup code for a secret (the short alphanumeric TeamVault key,
// e.g. "MOPmQL"), not the secret value itself.
type Key string

// String returns the key as a plain string.
func (k Key) String() string {
	return string(k)
}

// Secret is a stored secret and its current-revision data. In the
// TeamVault-compatible API the metadata (Username, URL) and the revision data
// (Password, File) are served from two different endpoints, but Lockbox stores
// them together as one record.
type Secret struct {
	// Username is the account name associated with the secret; may be empty.
	Username string
	// URL is the resource the secret grants access to; may be empty.
	URL string
	// Password is the secret value; may be empty when the secret is file-only.
	Password string
	// File is the base64-encoded file payload; may be empty.
	File string
	// Name is the human-readable secret name (TeamVault name).
	Name string
	// Description is a free-text description of the secret; may be empty.
	Description string
	// ContentType is the TeamVault content type, either ContentTypePassword or
	// ContentTypeFile.
	ContentType string
}

// ContentTypePassword is the TeamVault content type for a password secret.
const ContentTypePassword = "password"

// ContentTypeFile is the TeamVault content type for a file secret.
const ContentTypeFile = "file"
